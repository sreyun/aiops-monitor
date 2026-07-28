package com.aiops.monitor.ui.viewmodel

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.aiops.monitor.data.ApiClient
import com.aiops.monitor.data.models.Host
import com.aiops.monitor.data.models.HardwareSnapshotWrap
import com.aiops.monitor.data.models.HyperVInventory
import com.aiops.monitor.data.models.NetFlowRecord
import com.aiops.monitor.data.models.NetFlowIPHistoryResponse
import com.aiops.monitor.data.models.NetFlowSummaryItem
import com.aiops.monitor.data.models.SNMPDevice
import com.aiops.monitor.data.models.SNMPInterface
import com.aiops.monitor.data.models.SNMPInterfaceHistoryResponse
import com.aiops.monitor.data.models.SNMPTrapEvent
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch

/**
 * 硬件监控。像 web 端一样：自己拉主机列表，再并发拉每台的 Redfish/OceanStor 快照，
 * 汇总成一个按「异常优先」排序的设备列表。
 */
class HardwareViewModel : ViewModel() {
    data class HwItem(val host: Host, val wrap: HardwareSnapshotWrap, val online: Boolean)

    private val _items = MutableStateFlow<List<HwItem>>(emptyList())
    val items: StateFlow<List<HwItem>> = _items

    private val _loading = MutableStateFlow(false)
    val loading: StateFlow<Boolean> = _loading

    private val _error = MutableStateFlow<String?>(null)
    val error: StateFlow<String?> = _error

    fun load() {
        if (!ApiClient.isInitialized()) { _error.value = "API 未初始化，请先登录"; return }
        if (_loading.value) return
        _loading.value = true
        _error.value = null
        viewModelScope.launch {
            try {
                val hosts = ApiClient.api.hosts()
                // 不过滤离线主机：BMC 是带外通道，主机宕机时的硬件数据恰恰最有价值。
                val items = coroutineScope {
                    hosts.map { h ->
                        async {
                            try {
                                ApiClient.api.hardwareHealth(h.id).snapshots.orEmpty().map {
                                    HwItem(h, it, online = h.online)
                                }
                            } catch (_: Exception) { emptyList() }
                        }
                    }.awaitAll().flatten()
                }
                // Critical > Warning > OK 优先
                val order = mapOf("Critical" to 0, "Warning" to 1, "OK" to 2)
                _items.value = items.sortedBy { order[it.wrap.health] ?: 3 }
                if (items.isEmpty()) _error.value = null
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _error.value = e.message ?: "加载失败"
            } finally {
                _loading.value = false
            }
        }
    }
}

/**
 * 网络流量。Top-N 排行（可切维度）来自 PG 聚合，Flow 明细来自永久保留的分区表。
 */
class NetflowViewModel : ViewModel() {
    private val _summary = MutableStateFlow<List<NetFlowSummaryItem>>(emptyList())
    val summary: StateFlow<List<NetFlowSummaryItem>> = _summary

    private val _flows = MutableStateFlow<List<NetFlowRecord>>(emptyList())
    val flows: StateFlow<List<NetFlowRecord>> = _flows

    private val _loading = MutableStateFlow(false)
    val loading: StateFlow<Boolean> = _loading

    private val _error = MutableStateFlow<String?>(null)
    val error: StateFlow<String?> = _error

    private val _ipHistory = MutableStateFlow<NetFlowIPHistoryResponse?>(null)
    val ipHistory: StateFlow<NetFlowIPHistoryResponse?> = _ipHistory

    private val _historyLoading = MutableStateFlow(false)
    val historyLoading: StateFlow<Boolean> = _historyLoading

    fun load(hostId: String, dimension: String, range: String) {
        if (!ApiClient.isInitialized() || hostId.isBlank()) return
        if (_loading.value) return
        _loading.value = true
        _error.value = null
        viewModelScope.launch {
            try {
                coroutineScope {
                    val sum = async {
                        try { ApiClient.api.netflowSummary(hostId, dimension, range, 10).summary.orEmpty() }
                        catch (_: Exception) { emptyList() }
                    }
                    val fl = async {
                        try { ApiClient.api.netflowFlows(hostId, 100, range = range).flows.orEmpty() }
                        catch (_: Exception) { emptyList() }
                    }
                    _summary.value = sum.await()
                    _flows.value = fl.await()
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _error.value = e.message ?: "加载失败"
            } finally {
                _loading.value = false
            }
        }
    }

    fun loadIPHistory(hostId: String, ip: String, dimension: String, range: ChartTimeRange) {
        if (!ApiClient.isInitialized() || hostId.isBlank() || ip.isBlank()) return
        _ipHistory.value = null
        _historyLoading.value = true
        viewModelScope.launch {
            try {
                val to = System.currentTimeMillis() / 1000
                val from = to - range.minutes.toLong() * 60
                _ipHistory.value = ApiClient.api.netflowIPHistory(hostId, ip, dimension, from, to)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _error.value = e.message ?: "IP 历史加载失败"
            } finally {
                _historyLoading.value = false
            }
        }
    }

    fun loadRelatedFlows(hostId: String, ip: String, dimension: String, range: ChartTimeRange) {
        if (!ApiClient.isInitialized() || hostId.isBlank() || ip.isBlank()) return
        _loading.value = true
        viewModelScope.launch {
            try {
                val to = System.currentTimeMillis() / 1000
                val from = to - range.minutes.toLong() * 60
                _flows.value = ApiClient.api.netflowFlows(
                    host = hostId, limit = 100, filter = "$dimension:$ip", from = from, to = to
                ).flows.orEmpty()
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _error.value = e.message ?: "相关 Flow 加载失败"
            } finally {
                _loading.value = false
            }
        }
    }
}

/**
 * Hyper-V 虚拟机。拉取每台物理宿主机的 guest 清单，按「异常/运行优先」交由 UI 分组展示。
 */
class HyperVViewModel : ViewModel() {
    private val _inventories = MutableStateFlow<List<HyperVInventory>>(emptyList())
    val inventories: StateFlow<List<HyperVInventory>> = _inventories

    private val _loading = MutableStateFlow(false)
    val loading: StateFlow<Boolean> = _loading

    private val _error = MutableStateFlow<String?>(null)
    val error: StateFlow<String?> = _error

    fun load() {
        if (!ApiClient.isInitialized()) { _error.value = "API 未初始化，请先登录"; return }
        if (_loading.value) return
        _loading.value = true
        _error.value = null
        viewModelScope.launch {
            try {
                _inventories.value = ApiClient.api.hypervList().inventories.orEmpty()
                    .sortedByDescending { it.guest_count }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _error.value = e.message ?: "加载失败"
            } finally {
                _loading.value = false
            }
        }
    }
}

/**
 * SNMP 网络设备。按主机(采集端 Agent)拉取被轮询设备快照 + 最近 Trap 事件。
 */
class SNMPViewModel : ViewModel() {
    private val _devices = MutableStateFlow<List<SNMPDevice>>(emptyList())
    val devices: StateFlow<List<SNMPDevice>> = _devices

    private val _traps = MutableStateFlow<List<SNMPTrapEvent>>(emptyList())
    val traps: StateFlow<List<SNMPTrapEvent>> = _traps

    private val _loading = MutableStateFlow(false)
    val loading: StateFlow<Boolean> = _loading

    private val _error = MutableStateFlow<String?>(null)
    val error: StateFlow<String?> = _error

    private val _interfaceHistory = MutableStateFlow<SNMPInterfaceHistoryResponse?>(null)
    val interfaceHistory: StateFlow<SNMPInterfaceHistoryResponse?> = _interfaceHistory

    private val _historyLoading = MutableStateFlow(false)
    val historyLoading: StateFlow<Boolean> = _historyLoading

    fun load(hostId: String) {
        if (!ApiClient.isInitialized() || hostId.isBlank()) return
        if (_loading.value) return
        _loading.value = true
        _error.value = null
        viewModelScope.launch {
            try {
                coroutineScope {
                    val dev = async {
                        try { ApiClient.api.snmpList(hostId).devices.orEmpty() } catch (_: Exception) { emptyList() }
                    }
                    val tr = async {
                        try { ApiClient.api.snmpTraps(hostId, 50).traps.orEmpty() } catch (_: Exception) { emptyList() }
                    }
                    _devices.value = dev.await()
                    _traps.value = tr.await()
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _error.value = e.message ?: "加载失败"
            } finally {
                _loading.value = false
            }
        }
    }

    fun loadInterfaceHistory(
        hostId: String,
        device: SNMPDevice,
        iface: SNMPInterface,
        range: ChartTimeRange
    ) {
        if (!ApiClient.isInitialized() || hostId.isBlank()) return
        _interfaceHistory.value = null
        _historyLoading.value = true
        viewModelScope.launch {
            try {
                val to = System.currentTimeMillis() / 1000
                val from = to - range.minutes.toLong() * 60
                _interfaceHistory.value = ApiClient.api.snmpInterfaceHistory(
                    host = hostId,
                    device = device.device_name,
                    deviceIp = device.device_ip,
                    ifindex = iface.index,
                    ifname = iface.name,
                    from = from,
                    to = to
                )
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _error.value = e.message ?: "接口历史加载失败"
            } finally {
                _historyLoading.value = false
            }
        }
    }
}
