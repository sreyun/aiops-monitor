package com.aiops.monitor.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.*
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Fullscreen
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.DialogProperties
import androidx.lifecycle.viewmodel.compose.viewModel
import com.aiops.monitor.data.models.ContentAuditEvent
import com.aiops.monitor.data.models.SNMPDevice
import com.aiops.monitor.data.models.SNMPInterface
import com.aiops.monitor.data.models.SNMPTrapEvent
import com.aiops.monitor.data.models.NetFlowDrillItem
import com.aiops.monitor.ui.components.SectionCard
import com.aiops.monitor.data.models.NetFlowIPHistoryResponse
import com.aiops.monitor.data.models.SNMPInterfaceHistoryResponse
import com.aiops.monitor.ui.components.*
import com.aiops.monitor.ui.viewmodel.ContentAuditViewModel
import com.aiops.monitor.ui.viewmodel.DataHostsViewModel
import com.aiops.monitor.ui.viewmodel.HostsViewModel
import com.aiops.monitor.ui.viewmodel.NetflowViewModel
import com.aiops.monitor.ui.viewmodel.SNMPViewModel
import com.aiops.monitor.ui.viewmodel.ChartTimeRange

private val NfAccent = Color(0xFF4F7FFF)
private val NfGreen = Color(0xFF00A86B)
private val NfRed = Color(0xFFEF4444)
private val NfOrange = Color(0xFFF59E0B)
private val NfGray = Color(0xFF8A93A3)

private fun fmtBytes(b: Long): String {
    if (b <= 0) return "0 B"
    val u = arrayOf("B", "KB", "MB", "GB", "TB")
    var v = b.toDouble(); var i = 0
    while (v >= 1024 && i < u.size - 1) { v /= 1024; i++ }
    return if (i == 0) "$b B" else String.format("%.1f %s", v, u[i])
}

private fun fmtBps(bps: Double): String {
    if (bps <= 0) return "0"
    val u = arrayOf("bps", "Kbps", "Mbps", "Gbps", "Tbps"); var v = bps; var i = 0
    while (v >= 1000 && i < u.size - 1) { v /= 1000; i++ }
    return "%.1f %s".format(v, u[i])
}

private fun fmtSpeed(bps: Long): String {
    if (bps <= 0) return "—"
    val u = arrayOf("bps", "Kbps", "Mbps", "Gbps", "Tbps"); var v = bps.toDouble(); var i = 0
    while (v >= 1000 && i < u.size - 1) { v /= 1000; i++ }
    return "%.0f %s".format(v, u[i])
}

private fun fmtUptimeSec(sec: Double): String {
    val s = sec.toLong(); if (s <= 0) return "—"
    val d = s / 86400; val h = (s % 86400) / 3600; val m = (s % 3600) / 60
    return when { d > 0 -> "${d}天${h}时"; h > 0 -> "${h}时${m}分"; else -> "${m}分" }
}

private fun protoName(p: Int): String = when (p) { 6 -> "TCP"; 17 -> "UDP"; 1 -> "ICMP"; else -> "$p" }

/** 接口异常度：3=admin up 但 oper down（链路断），2=利用率≥80%，0=正常。 */
private fun ifBad(i: SNMPInterface): Int {
    if (i.admin_status == 1 && !i.oper_up) return 3
    if (maxOf(i.in_util_percent, i.out_util_percent) >= 80) return 2
    return 0
}

/**
 * 网络中枢（基础设施「网络」子标签）：共享主机选择 + 分段 [设备 / 流量 / Trap / 内容审计]。
 * 设备=SNMP 网络设备与接口；流量=NetFlow Top-N 与 Flow 明细；Trap=SNMP 陷阱事件；内容审计=HTTP 请求/响应审计。
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun NetworkContent(modifier: Modifier = Modifier, refreshSignal: Int = 0) {
    val hostsVm: HostsViewModel = viewModel()
    val hosts by hostsVm.hosts.collectAsState()

    val dataHostsVm: DataHostsViewModel = viewModel()
    val netflowHosts by dataHostsVm.netflowHosts.collectAsState()
    val snmpHosts by dataHostsVm.snmpHosts.collectAsState()
    val contentAuditHosts by dataHostsVm.contentAuditHosts.collectAsState()

    var host by remember { mutableStateOf("") }
    var seg by remember { mutableIntStateOf(0) } // 0 设备 / 1 流量 / 2 Trap / 3 内容审计
    var hostMenu by remember { mutableStateOf(false) }
    var showAllHosts by remember { mutableStateOf(false) }

    // 根据当前分段决定使用哪个数据主机集合（必须在 LaunchedEffect 之前定义）
    val dataHostIds = when (seg) {
        0 -> snmpHosts
        1 -> netflowHosts
        3 -> contentAuditHosts
        else -> snmpHosts // Trap 也用 snmp 的主机
    }

    LaunchedEffect(Unit) { hostsVm.load(); dataHostsVm.load() }
    // 等数据主机加载完成后，自动选择第一个有数据的主机（解决 filteredHosts 初始为空的问题）
    LaunchedEffect(hosts, dataHostIds) {
        if (host.isBlank() || host !in dataHostIds) {
            hosts.firstOrNull { it.online && it.id in dataHostIds }?.let { host = it.id }
        }
    }

    // 过滤：有数据的主机或显示全部
    val filteredHosts = remember(hosts, dataHostIds, showAllHosts) {
        if (showAllHosts) hosts.filter { it.online }
        else hosts.filter { it.online && it.id in dataHostIds }
    }

    val hostLabel = hosts.firstOrNull { it.id == host }?.hostname ?: "选择主机"

    Column(modifier.fillMaxSize().background(MaterialTheme.colorScheme.background)) {
        Column(Modifier.padding(horizontal = 16.dp, vertical = 6.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Box {
                OutlinedButton(onClick = { hostMenu = true }, modifier = Modifier.fillMaxWidth()) {
                    Text(hostLabel, maxLines = 1, overflow = TextOverflow.Ellipsis)
                }
                DropdownMenu(expanded = hostMenu, onDismissRequest = { hostMenu = false }) {
                    filteredHosts.forEach { h ->
                        DropdownMenuItem(text = { Text(h.hostname) }, onClick = { host = h.id; hostMenu = false })
                    }
                    if (filteredHosts.isEmpty()) {
                        DropdownMenuItem(
                            text = { Text("暂无有数据的主机", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant) },
                            onClick = { hostMenu = false },
                            enabled = false
                        )
                    }
                }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                listOf("设备", "流量", "Trap", "内容审计").forEachIndexed { i, label ->
                    FilterChip(selected = seg == i, onClick = { seg = i }, label = { Text(label, fontWeight = FontWeight.Medium) }, colors = chipColors())
                }
            }
            // 显示全部主机开关
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.End) {
                Text(
                    if (showAllHosts) "显示全部" else "仅显示有数据",
                    fontSize = 11.sp,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
                Spacer(Modifier.width(4.dp))
                Switch(
                    checked = showAllHosts,
                    onCheckedChange = { showAllHosts = it },
                    modifier = Modifier.height(20.dp)
                )
            }
        }

        Box(Modifier.weight(1f).fillMaxWidth()) {
            when (seg) {
                0 -> SnmpDevicesView(host, refreshSignal)
                1 -> NetflowTrafficView(host, refreshSignal)
                2 -> SnmpTrapsView(host, refreshSignal)
                else -> ContentAuditView(host, refreshSignal)
            }
        }
    }
}

/* ───────────────────────── 设备（SNMP） ───────────────────────── */

/** 端口状态筛选：与 Web 端 snIfFilter 对齐，默认只看在线。 */
private enum class SnmpIfFilter(val label: String) {
    All("全部"), Up("在线"), Down("离线");
}

private fun snmpIfMatches(i: SNMPInterface, filter: SnmpIfFilter): Boolean = when (filter) {
    SnmpIfFilter.Up -> i.oper_up
    SnmpIfFilter.Down -> !i.oper_up
    SnmpIfFilter.All -> true
}

@Composable
private fun SnmpDevicesView(host: String, refreshSignal: Int) {
    val vm: SNMPViewModel = viewModel()
    val devices by vm.devices.collectAsState()
    val loading by vm.loading.collectAsState()
    val history by vm.interfaceHistory.collectAsState()
    val historyLoading by vm.historyLoading.collectAsState()
    var selected by remember { mutableStateOf<Pair<SNMPDevice, SNMPInterface>?>(null) }
    var historyRange by remember { mutableStateOf(ChartTimeRange.ONE_HOUR) }
    var ifFilter by remember { mutableStateOf(SnmpIfFilter.Up) }
    var showAi by remember { mutableStateOf(false) }

    LaunchedEffect(host) { if (host.isNotBlank()) vm.load(host) }
    LaunchedEffect(refreshSignal) { if (refreshSignal > 0 && host.isNotBlank()) vm.load(host) }

    when {
        loading && devices.isEmpty() -> LoadingBox(Modifier.fillMaxSize())
        devices.isEmpty() -> StateBox("暂无 SNMP 网络设备数据\n请在被控端配置 SNMP 轮询目标（交换机/路由器）后上报", Modifier.fillMaxSize())
        else -> LazyColumn(
            Modifier.fillMaxSize(),
            contentPadding = PaddingValues(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            item {
                Row(
                    Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Text("端口", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    SnmpIfFilter.entries.forEach { f ->
                        FilterChip(
                            selected = ifFilter == f,
                            onClick = { ifFilter = f },
                            label = { Text(f.label, fontSize = 12.sp) },
                            colors = chipColors()
                        )
                    }
                    Spacer(Modifier.weight(1f))
                    TextButton(onClick = { showAi = true }, enabled = devices.isNotEmpty()) { Text("AI 诊断") }
                }
            }
            itemsIndexed(devices, key = { i, d -> "$i#${d.device_ip}" }) { _, d ->
                SnmpDeviceCard(
                    d = d,
                    ifFilter = ifFilter,
                    onFilterChange = { ifFilter = it },
                    onHistory = { iface ->
                        selected = d to iface
                        historyRange = ChartTimeRange.ONE_HOUR
                        vm.loadInterfaceHistory(host, d, iface, historyRange)
                    }
                )
            }
        }
    }

    selected?.let { (device, iface) ->
        SnmpInterfaceHistoryDialog(
            device = device,
            iface = iface,
            history = history,
            range = historyRange,
            loading = historyLoading,
            onRangeChange = {
                historyRange = it
                vm.loadInterfaceHistory(host, device, iface, it)
            },
            onDismiss = { selected = null }
        )
    }
    if (showAi && devices.isNotEmpty()) {
        val ctx = buildString {
            appendLine("SNMP 主机：$host · 设备数 ${devices.size}")
            devices.take(12).forEach { d ->
                val ifs = d.snapshot?.interfaces.orEmpty()
                appendLine("- ${d.device_ip} reachable=${d.reachable} if=${ifs.size} up=${ifs.count { it.oper_up }}")
                ifs.filter { ifBad(it) > 0 }.take(8).forEach { i ->
                    appendLine("  · if${i.index} ${i.name.ifBlank { i.descr }} admin=${i.admin_status} oper_up=${i.oper_up}")
                }
            }
        }
        AiAssistSheet(
            title = "AI 网络设备诊断",
            task = "snmp_diagnosis",
            context = ctx.take(14000),
            onDismiss = { showAi = false }
        )
    }
}

@Composable
private fun SnmpDeviceCard(
    d: SNMPDevice,
    ifFilter: SnmpIfFilter,
    onFilterChange: (SnmpIfFilter) -> Unit,
    onHistory: (SNMPInterface) -> Unit
) {
    val sys = d.snapshot?.system
    val ifs = d.snapshot?.interfaces.orEmpty()
    val up = ifs.count { it.oper_up }
    val down = ifs.size - up
    val bad = ifs.count { ifBad(it) > 0 }
    val visible = ifs.filter { snmpIfMatches(it, ifFilter) }
    val statusColor = if (!d.reachable) NfRed else if (bad > 0) NfOrange else NfGreen

    Card(
        Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(14.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 1.dp)
    ) {
        Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min)) {
            Box(Modifier.width(4.dp).fillMaxHeight().background(statusColor))
            Column(Modifier.weight(1f).padding(14.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    StatusDot(statusColor, size = 9.dp)
                    Column(Modifier.weight(1f)) {
                        Text(d.device_name.ifBlank { sys?.name ?: "设备" }, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Text(listOf(d.device_ip, sys?.name ?: "").filter { it.isNotBlank() }.joinToString(" · "), fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    }
                    if (!d.reachable) StatusPill("不可达", NfRed)
                }
                // 统计可点：与 Web 端 sn-stat 一致，快速切到全部 / 在线 / 离线
                Row(
                    Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    SnmpStatChip("接口 ${ifs.size}", selected = ifFilter == SnmpIfFilter.All) { onFilterChange(SnmpIfFilter.All) }
                    SnmpStatChip("UP $up", selected = ifFilter == SnmpIfFilter.Up, accent = NfGreen) { onFilterChange(SnmpIfFilter.Up) }
                    SnmpStatChip(
                        "DOWN $down",
                        selected = ifFilter == SnmpIfFilter.Down,
                        accent = if (down > 0) NfRed else MaterialTheme.colorScheme.onSurfaceVariant
                    ) { onFilterChange(SnmpIfFilter.Down) }
                    Text("运行 ${fmtUptimeSec(sys?.uptime_sec ?: 0.0)}", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                if (ifs.isNotEmpty()) {
                    HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant, thickness = 0.5.dp)
                    if (visible.isEmpty()) {
                        Text("无匹配接口", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(vertical = 8.dp))
                    } else {
                        visible.sortedByDescending { ifBad(it) }.forEach { i ->
                            InterfaceRow(i) { onHistory(i) }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun SnmpStatChip(
    label: String,
    selected: Boolean,
    accent: Color = MaterialTheme.colorScheme.onSurface,
    onClick: () -> Unit
) {
    val bg = if (selected) accent.copy(alpha = 0.14f) else MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.55f)
    val fg = if (selected) accent else MaterialTheme.colorScheme.onSurfaceVariant
    Text(
        label,
        fontSize = 11.sp,
        fontWeight = if (selected) FontWeight.SemiBold else FontWeight.Medium,
        color = fg,
        modifier = Modifier
            .clip(RoundedCornerShape(8.dp))
            .background(bg)
            .clickable(onClick = onClick)
            .padding(horizontal = 10.dp, vertical = 5.dp)
    )
}

@Composable
private fun InterfaceRow(i: SNMPInterface, onHistory: () -> Unit) {
    val bad = ifBad(i)
    val badge = if (i.oper_up) "UP" else "DOWN"
    val badgeColor = when { !i.oper_up -> NfRed; bad == 2 -> NfOrange; else -> NfGreen }
    val util = maxOf(i.in_util_percent, i.out_util_percent)
    val bg = when (bad) { 3 -> NfRed.copy(alpha = 0.06f); 2 -> NfOrange.copy(alpha = 0.06f); else -> Color.Transparent }
    Column(
        Modifier.fillMaxWidth().clip(RoundedCornerShape(8.dp)).background(bg).clickable(onClick = onHistory).padding(vertical = 6.dp, horizontal = 6.dp),
        verticalArrangement = Arrangement.spacedBy(2.dp)
    ) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            StatusPill(badge, badgeColor)
            Text(i.name.ifBlank { i.descr.ifBlank { "if${i.index}" } }, fontSize = 12.sp, fontWeight = FontWeight.Medium, color = MaterialTheme.colorScheme.onSurface, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
            Text(fmtSpeed(i.speed_bps), fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            Text("历史 ›", fontSize = 10.sp, color = NfAccent)
        }
        if (i.oper_up) {
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Text("↓ ${fmtBps(i.in_bps)}", fontSize = 11.sp, color = NfAccent)
                Text("↑ ${fmtBps(i.out_bps)}", fontSize = 11.sp, color = NfGreen)
                if (i.rate_valid) Text("利用率 ${util.toInt()}%", fontSize = 11.sp, color = if (util >= 80) NfOrange else MaterialTheme.colorScheme.onSurfaceVariant)
                val err = i.in_err_pps + i.out_err_pps + i.in_discard_pps + i.out_discard_pps
                if (err > 0) Text("错误/丢包 %.1f/s".format(err), fontSize = 11.sp, color = NfRed)
            }
        }
        if (i.alias.isNotBlank()) Text(i.alias, fontSize = 10.sp, color = NfGray, maxLines = 1, overflow = TextOverflow.Ellipsis)
    }
}

private data class HistoryMetric(val key: String, val title: String, val unit: String, val color: Color)

private val snmpHistoryMetrics = listOf(
    HistoryMetric("in_bps", "入向流量", "bps", NfAccent),
    HistoryMetric("out_bps", "出向流量", "bps", NfGreen),
    HistoryMetric("in_util", "入向利用率", "%", Color(0xFF8B5CF6)),
    HistoryMetric("out_util", "出向利用率", "%", NfOrange),
    HistoryMetric("in_err_pps", "入向错误", "次/s", NfRed),
    HistoryMetric("out_err_pps", "出向错误", "次/s", Color(0xFFDC2626)),
    HistoryMetric("in_disc_pps", "入向丢弃", "包/s", Color(0xFFEC4899)),
    HistoryMetric("out_disc_pps", "出向丢弃", "包/s", Color(0xFFBE185D)),
    HistoryMetric("oper_up", "接口状态（1=UP）", "", NfGreen),
    HistoryMetric("speed_bps", "协商速率", "bps", NfAccent)
)

private fun SNMPInterfaceHistoryResponse.points(key: String): Pair<List<Double>, List<Long>> {
    val points = series?.get(key).orEmpty().flatMap { it.values.orEmpty() }.mapNotNull { raw ->
        if (raw.size < 2) return@mapNotNull null
        val ts = (raw[0] as? Number)?.toLong() ?: raw[0].toString().toDoubleOrNull()?.toLong() ?: return@mapNotNull null
        val value = (raw[1] as? Number)?.toDouble() ?: raw[1].toString().toDoubleOrNull() ?: return@mapNotNull null
        ts to value
    }.sortedBy { it.first }
    return points.map { it.second } to points.map { it.first }
}

@Composable
private fun SnmpInterfaceHistoryDialog(
    device: SNMPDevice,
    iface: SNMPInterface,
    history: SNMPInterfaceHistoryResponse?,
    range: ChartTimeRange,
    loading: Boolean,
    onRangeChange: (ChartTimeRange) -> Unit,
    onDismiss: () -> Unit
) {
    val title = iface.name.ifBlank { iface.descr.ifBlank { "if${iface.index}" } }
    var fullscreen by remember { mutableStateOf<ChartFullscreen?>(null) }
    AlertDialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false),
        modifier = Modifier.fillMaxSize(),
        confirmButton = {},
        title = null,
        text = {
            Column(Modifier.fillMaxSize(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        Text("$title · 接口历史", fontWeight = FontWeight.Bold, fontSize = 17.sp)
                        Text("${device.device_name} · ${device.device_ip} · ifIndex ${iface.index}", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    IconButton(onClick = onDismiss) { Icon(Icons.Default.Close, "关闭") }
                }
                Row(verticalAlignment = Alignment.CenterVertically) {
                    TimeRangeSelector(range, onRangeChange, Modifier.weight(1f))
                    if (loading) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
                }
                LazyColumn(Modifier.fillMaxWidth().weight(1f), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    snmpHistoryMetrics.forEach { metric ->
                        val (values, timestamps) = history?.points(metric.key) ?: (emptyList<Double>() to emptyList())
                        if (values.isNotEmpty()) {
                            item(metric.key) {
                                HistoryChartCard(metric.title, values, timestamps, metric.unit, metric.color) {
                                    fullscreen = ChartFullscreen(metric.title, values, timestamps, metric.unit, metric.color)
                                }
                            }
                        }
                    }
                    if (!loading && snmpHistoryMetrics.all { history?.points(it.key)?.first.isNullOrEmpty() }) {
                        item { StateBox("当前时间范围暂无接口历史数据", Modifier.fillMaxWidth().height(140.dp)) }
                    }
                }
            }
        },
        containerColor = MaterialTheme.colorScheme.surface
    )
    fullscreen?.let { fs ->
        FullscreenChartDialog(
            title = fs.title,
            data = fs.values,
            timestamps = fs.timestamps,
            color = fs.color,
            yLabel = fs.unit,
            onDismiss = { fullscreen = null }
        )
    }
}

/* ───────────────────────── 流量（NetFlow） ───────────────────────── */

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun NetflowTrafficView(host: String, refreshSignal: Int) {
    val vm: NetflowViewModel = viewModel()
    val summary by vm.summary.collectAsState()
    val flows by vm.flows.collectAsState()
    val loading by vm.loading.collectAsState()
    val ipHistory by vm.ipHistory.collectAsState()
    val historyLoading by vm.historyLoading.collectAsState()

    var dimension by remember { mutableStateOf("dst_ip") }
    var range by remember { mutableStateOf(ChartTimeRange.ONE_HOUR) }
    var selectedIP by remember { mutableStateOf<Pair<String, String>?>(null) }
    var showAi by remember { mutableStateOf(false) }

    LaunchedEffect(host, dimension, range) { if (host.isNotBlank()) vm.load(host, dimension, range.apiRange()) }
    LaunchedEffect(refreshSignal) { if (refreshSignal > 0 && host.isNotBlank()) vm.load(host, dimension, range.apiRange()) }

    Column(Modifier.fillMaxSize()) {
        Column(Modifier.padding(horizontal = 16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
                listOf("dst_ip" to "目的IP", "src_ip" to "源IP", "dst_port" to "目的端口", "src_port" to "源端口", "protocol" to "协议").forEach { (k, label) ->
                    FilterChip(selected = dimension == k, onClick = { dimension = k }, label = { Text(label) })
                }
                TextButton(onClick = { showAi = true }, enabled = summary.isNotEmpty() || flows.isNotEmpty()) { Text("AI 分析") }
            }
            TimeRangeSelector(range, { range = it })
        }
        Spacer(Modifier.height(8.dp))

        if (loading && summary.isEmpty() && flows.isEmpty()) {
            LoadingBox(Modifier.fillMaxSize())
            return@Column
        }

        LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            item {
                SectionCard(title = "流量排行 Top ${summary.size}") {
                    if (summary.isEmpty()) {
                        Text("暂无流量数据", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    } else {
                        val max = summary.firstOrNull()?.bytes ?: 1L
                        summary.forEach { s ->
                            val canDrill = dimension == "src_ip" || dimension == "dst_ip"
                            Column(
                                Modifier.fillMaxWidth()
                                    .then(if (canDrill) Modifier.clickable {
                                        selectedIP = s.key to dimension
                                        vm.loadIPHistory(host, s.key, dimension, range)
                                    } else Modifier)
                                    .padding(vertical = 5.dp)
                            ) {
                                Row(Modifier.fillMaxWidth(), Arrangement.SpaceBetween) {
                                    Text(s.key.ifBlank { "-" }, fontSize = 12.sp, fontWeight = FontWeight.Medium,
                                        color = MaterialTheme.colorScheme.onSurface, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
                                    if (canDrill) Text("历史 ›", fontSize = 10.sp, color = NfAccent, modifier = Modifier.padding(end = 8.dp))
                                    Text(fmtBytes(s.bytes), fontSize = 12.sp, color = NfAccent, fontWeight = FontWeight.Bold)
                                }
                                Spacer(Modifier.height(3.dp))
                                Box(Modifier.fillMaxWidth().height(6.dp).clip(RoundedCornerShape(3.dp)).background(MaterialTheme.colorScheme.surfaceVariant)) {
                                    val frac = (s.bytes.toDouble() / max).coerceIn(0.02, 1.0).toFloat()
                                    Box(Modifier.fillMaxWidth(frac).height(6.dp).clip(RoundedCornerShape(3.dp)).background(NfAccent.copy(alpha = 0.75f)))
                                }
                            }
                        }
                    }
                }
            }
            item { Text("Flow 明细（${flows.size}）", fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface) }
            if (flows.isEmpty()) {
                item { Text("暂无 Flow 记录", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant) }
            } else {
                itemsIndexed(flows, key = { i, _ -> i }) { _, f ->
                    Card(Modifier.fillMaxWidth(), shape = RoundedCornerShape(10.dp), colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface), elevation = CardDefaults.cardElevation(defaultElevation = 0.dp)) {
                        Column(Modifier.padding(10.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                            Row(Modifier.fillMaxWidth(), Arrangement.SpaceBetween, Alignment.CenterVertically) {
                                Row(Modifier.weight(1f), verticalAlignment = Alignment.CenterVertically) {
                                    Text(
                                        "${f.src_ip}:${f.src_port}",
                                        fontSize = 12.sp,
                                        fontWeight = FontWeight.Medium,
                                        color = NfAccent,
                                        maxLines = 1,
                                        modifier = Modifier.clickable {
                                            selectedIP = f.src_ip to "src_ip"
                                            vm.loadIPHistory(host, f.src_ip, "src_ip", range)
                                        }
                                    )
                                    Text(" → ", fontSize = 12.sp)
                                    Text(
                                        "${f.dst_ip}:${f.dst_port}",
                                        fontSize = 12.sp,
                                        fontWeight = FontWeight.Medium,
                                        color = NfAccent,
                                        maxLines = 1,
                                        modifier = Modifier.clickable {
                                            selectedIP = f.dst_ip to "dst_ip"
                                            vm.loadIPHistory(host, f.dst_ip, "dst_ip", range)
                                        }
                                    )
                                }
                                StatusPill(protoName(f.protocol), NfAccent)
                            }
                            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                                Text("↕ ${fmtBytes(f.bytes)}", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                Text("📦 ${f.packets}", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                if (f.source.isNotBlank()) Text(f.source, fontSize = 11.sp, color = NfGray)
                            }
                            // 目的地富化：域名 / 国家 / ASN 归属（服务端反查，内网时为空）
                            f.dst_enrich?.takeIf { it.host.isNotBlank() || it.org.isNotBlank() || it.country.isNotBlank() }?.let { en ->
                                Text(
                                    "→ " + listOf(en.host, en.country, en.org).filter { it.isNotBlank() }.joinToString(" · "),
                                    fontSize = 10.sp, color = NfAccent, maxLines = 1, overflow = TextOverflow.Ellipsis
                                )
                            }
                        }
                    }
                }
            }
        }
    }

    selectedIP?.let { (ip, ipDimension) ->
        NetFlowIPHistoryDialog(
            ip = ip,
            dimension = ipDimension,
            history = ipHistory,
            range = range,
            loading = historyLoading,
            onRangeChange = {
                range = it
                vm.loadIPHistory(host, ip, ipDimension, it)
            },
            onPeerClick = { peer ->
                val peerDimension = if (ipDimension == "src_ip") "dst_ip" else "src_ip"
                selectedIP = peer to peerDimension
                vm.loadIPHistory(host, peer, peerDimension, range)
            },
            onRelatedFlows = {
                vm.loadRelatedFlows(host, ip, ipDimension, range)
                selectedIP = null
            },
            onDismiss = { selectedIP = null }
        )
    }
    if (showAi && (summary.isNotEmpty() || flows.isNotEmpty())) {
        val ctx = buildString {
            appendLine("NetFlow 主机：$host 维度：$dimension 窗口：${range.label}")
            appendLine("排行 Top：")
            summary.take(15).forEach { appendLine("- ${it.key} ${it.bytes} bytes") }
            appendLine("Flow 样本：")
            flows.take(20).forEach {
                appendLine("- ${it.src_ip}:${it.src_port} → ${it.dst_ip}:${it.dst_port} proto=${it.protocol} bytes=${it.bytes}")
            }
        }
        AiAssistSheet(
            title = "AI 流量分析",
            task = "netflow_diagnosis",
            context = ctx.take(14000),
            onDismiss = { showAi = false }
        )
    }
}

private fun ChartTimeRange.apiRange(): String = when (this) {
    ChartTimeRange.ONE_HOUR -> "1h"
    ChartTimeRange.THREE_HOURS -> "3h"
    ChartTimeRange.SIX_HOURS -> "6h"
    ChartTimeRange.TWELVE_HOURS -> "12h"
    ChartTimeRange.ONE_DAY -> "24h"
    ChartTimeRange.THREE_DAYS -> "3d"
    ChartTimeRange.SEVEN_DAYS -> "7d"
    ChartTimeRange.FOURTEEN_DAYS -> "14d"
}

@Composable
private fun NetFlowIPHistoryDialog(
    ip: String,
    dimension: String,
    history: NetFlowIPHistoryResponse?,
    range: ChartTimeRange,
    loading: Boolean,
    onRangeChange: (ChartTimeRange) -> Unit,
    onPeerClick: (String) -> Unit,
    onRelatedFlows: () -> Unit,
    onDismiss: () -> Unit
) {
    val points = history?.points.orEmpty().sortedBy { it.timestamp }
    val timestamps = points.map { it.timestamp }
    var fullscreen by remember { mutableStateOf<ChartFullscreen?>(null) }
    AlertDialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false),
        modifier = Modifier.fillMaxSize(),
        confirmButton = {},
        title = null,
        text = {
            Column(Modifier.fillMaxSize(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        Text("$ip · 流量历史", fontWeight = FontWeight.Bold, fontSize = 17.sp)
                        Text(if (dimension == "src_ip") "作为源 IP" else "作为目的 IP", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    TextButton(onClick = onRelatedFlows) { Text("相关 Flow") }
                    IconButton(onClick = onDismiss) { Icon(Icons.Default.Close, "关闭") }
                }
                Row(verticalAlignment = Alignment.CenterVertically) {
                    TimeRangeSelector(range, onRangeChange, Modifier.weight(1f))
                    if (loading) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
                }
                LazyColumn(Modifier.fillMaxWidth().weight(1f), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    if (points.isNotEmpty()) {
                        item {
                            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                NetFlowKpi("流量", fmtBytes(points.sumOf { it.bytes }), Modifier.weight(1f))
                                NetFlowKpi("报文", points.sumOf { it.packets }.toString(), Modifier.weight(1f))
                                NetFlowKpi("会话", points.sumOf { it.flows }.toString(), Modifier.weight(1f))
                            }
                        }
                        item {
                            val v = points.map { it.bytes.toDouble() }
                            HistoryChartCard("流量趋势", v, timestamps, "B", NfAccent) { fullscreen = ChartFullscreen("流量趋势", v, timestamps, "B", NfAccent) }
                        }
                        item {
                            val v = points.map { it.packets.toDouble() }
                            HistoryChartCard("报文趋势", v, timestamps, "包", NfGreen) { fullscreen = ChartFullscreen("报文趋势", v, timestamps, "包", NfGreen) }
                        }
                        item {
                            val v = points.map { it.flows.toDouble() }
                            HistoryChartCard("会话活跃度", v, timestamps, "条", NfOrange) { fullscreen = ChartFullscreen("会话活跃度", v, timestamps, "条", NfOrange) }
                        }
                        item {
                            val v = points.map { it.peers.toDouble() }
                            HistoryChartCard("通信对端数量", v, timestamps, "个", Color(0xFF8B5CF6)) { fullscreen = ChartFullscreen("通信对端数量", v, timestamps, "个", Color(0xFF8B5CF6)) }
                        }
                        item {
                            val v = points.map { it.avg_packet_bytes }
                            HistoryChartCard("平均报文大小", v, timestamps, "B", Color(0xFF06B6D4)) { fullscreen = ChartFullscreen("平均报文大小", v, timestamps, "B", Color(0xFF06B6D4)) }
                        }
                    } else if (!loading) {
                        item { StateBox("当前时间范围暂无该 IP 的流量历史", Modifier.fillMaxWidth().height(140.dp)) }
                    }
                    history?.peers.orEmpty().takeIf { it.isNotEmpty() }?.let { rows ->
                        item { DrilldownSection("主要通信对端", rows, onClick = onPeerClick) }
                    }
                    history?.protocols.orEmpty().takeIf { it.isNotEmpty() }?.let { rows ->
                        item { DrilldownSection("协议分布", rows, keyLabel = { it.toIntOrNull()?.let(::protoName) ?: it }) }
                    }
                    history?.dst_ports.orEmpty().takeIf { it.isNotEmpty() }?.let { rows ->
                        item { DrilldownSection("目的端口分布", rows) }
                    }
                    history?.src_ports.orEmpty().takeIf { it.isNotEmpty() }?.let { rows ->
                        item { DrilldownSection("源端口分布", rows) }
                    }
                }
            }
        },
        containerColor = MaterialTheme.colorScheme.surface
    )
    fullscreen?.let { fs ->
        FullscreenChartDialog(
            title = fs.title,
            data = fs.values,
            timestamps = fs.timestamps,
            color = fs.color,
            yLabel = fs.unit,
            onDismiss = { fullscreen = null }
        )
    }
}

@Composable
private fun NetFlowKpi(label: String, value: String, modifier: Modifier = Modifier) {
    Column(modifier.clip(RoundedCornerShape(10.dp)).background(NfAccent.copy(alpha = 0.08f)).padding(9.dp)) {
        Text(label, fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Text(value, fontSize = 13.sp, fontWeight = FontWeight.Bold, color = NfAccent, maxLines = 1, overflow = TextOverflow.Ellipsis)
    }
}

/** 历史图表载荷，用于满屏放大查看。 */
private data class ChartFullscreen(
    val title: String,
    val values: List<Double>,
    val timestamps: List<Long>,
    val unit: String,
    val color: Color
)

/**
 * 统一的历史曲线卡片：标题行含满屏放大按钮，图表给足高度避免内容挤压。
 * 实现已抽到 ui.components.HistoryChartCard，此处保留别名以免调用处大面积改动。
 */
@Composable
private fun HistoryChartCard(
    title: String,
    values: List<Double>,
    timestamps: List<Long>,
    unit: String,
    color: Color,
    onExpand: () -> Unit
) {
    com.aiops.monitor.ui.components.HistoryChartCard(
        title = title,
        values = values,
        timestamps = timestamps,
        unit = unit,
        color = color,
        onExpand = onExpand,
    )
}

@Composable
private fun DrilldownSection(
    title: String,
    rows: List<NetFlowDrillItem>,
    keyLabel: (String) -> String = { it },
    onClick: ((String) -> Unit)? = null
) {
    SectionCard(title = title) {
        rows.forEach { row ->
            Row(
                Modifier.fillMaxWidth()
                    .then(if (onClick != null) Modifier.clickable { onClick(row.key) } else Modifier)
                    .padding(vertical = 6.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Column(Modifier.weight(1f)) {
                    Text(keyLabel(row.key), fontSize = 12.sp, fontWeight = FontWeight.Medium, color = if (onClick != null) NfAccent else MaterialTheme.colorScheme.onSurface)
                    row.enrich?.let { e ->
                        val detail = listOf(e.host, e.country, e.org).filter { it.isNotBlank() }.joinToString(" · ")
                        if (detail.isNotBlank()) Text(detail, fontSize = 9.sp, color = NfGray, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    }
                }
                Text(fmtBytes(row.bytes), fontSize = 11.sp, fontWeight = FontWeight.Bold, color = NfAccent)
                if (onClick != null) Text("  ›", color = NfAccent)
            }
        }
    }
}

/* ───────────────────────── Trap（SNMP 陷阱） ───────────────────────── */

@Composable
private fun SnmpTrapsView(host: String, refreshSignal: Int) {
    val vm: SNMPViewModel = viewModel()
    val traps by vm.traps.collectAsState()
    val loading by vm.loading.collectAsState()

    LaunchedEffect(host) { if (host.isNotBlank()) vm.load(host) }
    LaunchedEffect(refreshSignal) { if (refreshSignal > 0 && host.isNotBlank()) vm.load(host) }

    when {
        loading && traps.isEmpty() -> LoadingBox(Modifier.fillMaxSize())
        traps.isEmpty() -> StateBox("暂无 Trap 事件\n被控端在 :162 监听 SNMP Trap，设备主动上报后在此展示", Modifier.fillMaxSize())
        else -> LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            itemsIndexed(traps, key = { i, _ -> i }) { _, t -> TrapCard(t) }
        }
    }
}

@Composable
private fun TrapCard(t: SNMPTrapEvent) {
    val sevColor = when (t.severity.lowercase()) { "critical" -> NfRed; "warning" -> NfOrange; else -> NfAccent }
    val sevLabel = when (t.severity.lowercase()) { "critical" -> "严重"; "warning" -> "警告"; "info" -> "信息"; else -> t.severity }
    Card(Modifier.fillMaxWidth(), shape = RoundedCornerShape(12.dp), colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface), elevation = CardDefaults.cardElevation(defaultElevation = 1.dp)) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                StatusPill(sevLabel, sevColor)
                Text(t.source_ip.ifBlank { "未知来源" }, fontSize = 13.sp, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface, modifier = Modifier.weight(1f), maxLines = 1)
                Text("v${t.version}", fontSize = 10.sp, color = NfGray)
            }
            Text(t.trap_oid, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 2, overflow = TextOverflow.Ellipsis)
            val vbs = t.varbinds.orEmpty()
            if (vbs.isNotEmpty()) {
                Text(vbs.take(4).joinToString("  ") { "${it.oid.substringAfterLast('.')}=${it.value}" }, fontSize = 10.sp, color = NfGray, maxLines = 2, overflow = TextOverflow.Ellipsis)
            }
            Text(fmtRfc3339(t.received_at), fontSize = 10.sp, color = NfGray)
        }
    }
}

/* ───────────────────────── 内容审计 ───────────────────────── */

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ContentAuditView(host: String, refreshSignal: Int) {
    val vm: ContentAuditViewModel = viewModel()
    val events by vm.events.collectAsState()
    val loading by vm.loading.collectAsState()
    val error by vm.error.collectAsState()

    var query by remember { mutableStateOf("") }

    LaunchedEffect(host) { if (host.isNotBlank()) vm.load(host) }
    LaunchedEffect(refreshSignal) { if (refreshSignal > 0 && host.isNotBlank()) vm.load(host) }
    // 切换回内容审计 segment 时强制重新加载（ViewModel 被 Activity 级别持有，host 未变不会触发上面的 Effect）
    LaunchedEffect(Unit) { if (host.isNotBlank()) vm.load(host) }

    val filtered = remember(events, query) {
        val q = query.trim().lowercase()
        if (q.isEmpty()) events else events.filter { e ->
            listOf(e.src_ip, e.dst_ip, e.method, e.path, e.host, e.status.toString())
                .joinToString(" ").lowercase().let { hay -> q.split(" ").all { w -> hay.contains(w) } }
        }
    }

    Column(Modifier.fillMaxSize()) {
        OutlinedTextField(
            value = query,
            onValueChange = { query = it },
            modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
            placeholder = { Text("搜索 src_ip / dst_ip / method / path / status") },
            singleLine = true,
            keyboardOptions = KeyboardOptions(),
            shape = RoundedCornerShape(12.dp)
        )

        when {
            loading && events.isEmpty() -> LoadingBox(Modifier.fillMaxSize())
            error != null && events.isEmpty() -> StateBox("加载失败：$error", Modifier.fillMaxSize())
            events.isEmpty() -> StateBox("暂无内容审计数据\n请确认被控端已开启 HTTP 流量审计采集", Modifier.fillMaxSize())
            filtered.isEmpty() -> StateBox("没有匹配的审计事件", Modifier.fillMaxSize())
            else -> LazyColumn(
                Modifier.fillMaxSize(),
                contentPadding = PaddingValues(16.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                itemsIndexed(filtered, key = { i, _ -> i }) { _, e -> AuditEventCard(e) }
            }
        }
    }
}

@Composable
private fun AuditEventCard(e: ContentAuditEvent) {
    val methodColor = when (e.method.uppercase()) {
        "GET" -> NfGreen; "POST" -> NfAccent; "PUT" -> NfOrange; "DELETE" -> NfRed; else -> NfGray
    }
    val statusColor = when {
        e.status in 200..299 -> NfGreen
        e.status in 400..499 -> NfOrange
        e.status >= 500 -> NfRed
        else -> NfGray
    }
    val respSummary = e.resp_body.take(120).replace("\n", " ").replace("\r", "")

    Card(
        Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(12.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 1.dp)
    ) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            // 第一行：方法 + 状态码 + 时间
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                StatusPill(e.method.uppercase(), methodColor)
                StatusPill(e.status.toString(), statusColor)
                Text(e.path, fontSize = 12.sp, fontWeight = FontWeight.Medium, color = MaterialTheme.colorScheme.onSurface, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
                if (e.observed_at.isNotBlank()) Text(fmtRfc3339(e.observed_at), fontSize = 10.sp, color = NfGray)
            }
            // DLP 敏感命中（内容审计敏感数据扫描）
            if (e.sensitive.isNotBlank()) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    StatusPill("敏感", NfRed)
                    Text(e.sensitive, fontSize = 10.sp, color = NfRed, maxLines = 1, overflow = TextOverflow.Ellipsis)
                }
            }
            // 第二行：源IP → 目的IP:端口
            Text(
                "${e.src_ip} → ${e.dst_ip}:${e.dst_port}",
                fontSize = 11.sp,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
            // 第三行：主机 + Content-Type
            if (e.host.isNotBlank() || e.ctype.isNotBlank()) {
                Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    if (e.host.isNotBlank()) Text("Host: ${e.host}", fontSize = 10.sp, color = NfGray, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    if (e.ctype.isNotBlank()) Text(e.ctype, fontSize = 10.sp, color = NfGray, maxLines = 1, overflow = TextOverflow.Ellipsis)
                }
            }
            // 响应体摘要
            if (respSummary.isNotBlank()) {
                HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant.copy(alpha = 0.3f))
                Text(respSummary, fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 3, overflow = TextOverflow.Ellipsis)
            }
            // 截断标记
            if (e.req_truncated || e.resp_truncated) {
                Text("内容已截断", fontSize = 9.sp, color = NfOrange)
            }
        }
    }
}

// 服务端 PG 读取路径把时间列序列化为 RFC3339 字符串（observed_at / received_at），
// 不是 Unix 秒。解析带时区并换算到本机时区显示；解析失败时回退截取。
private fun fmtRfc3339(s: String): String {
    if (s.isBlank()) return ""
    return try {
        java.time.OffsetDateTime.parse(s)
            .atZoneSameInstant(java.time.ZoneId.systemDefault())
            .format(java.time.format.DateTimeFormatter.ofPattern("MM-dd HH:mm:ss"))
    } catch (_: Exception) {
        val t = s.replace('T', ' '); if (t.length >= 19) t.substring(5, 19) else t
    }
}