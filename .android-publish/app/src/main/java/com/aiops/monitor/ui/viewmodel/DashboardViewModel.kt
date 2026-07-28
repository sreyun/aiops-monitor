package com.aiops.monitor.ui.viewmodel

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.aiops.monitor.data.ApiClient
import com.aiops.monitor.data.models.Alert
import com.aiops.monitor.data.models.DashLogLine
import com.aiops.monitor.data.models.Dashboard
import com.aiops.monitor.data.models.DashboardMeta
import com.aiops.monitor.data.models.DashPanel
import com.aiops.monitor.data.models.PanelQueryReq
import com.aiops.monitor.data.models.VarValuesReq
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch

/** 一条序列（附带该目标的图例模板，用于渲染时格式化图例）。 */
data class DashSeries(val labels: Map<String, String>?, val points: List<List<Double>>?, val legendFmt: String)
data class DashValue(val labels: Map<String, String>?, val value: Double, val legendFmt: String)

/** 一个面板的查询结果状态（与 Web 端组件库对齐）。 */
sealed class PanelState {
    object Loading : PanelState()
    /** timeseries / graph / state-timeline / heatmap / stat（含 sparkline） */
    data class Range(val series: List<DashSeries>) : PanelState()
    /** gauge / bargauge / table / pie / bar / histogram */
    data class Instant(val values: List<DashValue>) : PanelState()
    data class Logs(val lines: List<DashLogLine>, val available: Boolean = true) : PanelState()
    data class Alerts(val items: List<Alert>) : PanelState()
    data class Err(val msg: String) : PanelState()
    object None : PanelState()
}

/** 仪表盘查看（只读）：列表 + 详情 + 模板变量 + 逐面板查询（与 Web 面板类型一致）。 */
class DashboardViewModel : ViewModel() {
    private val _dashboards = MutableStateFlow<List<DashboardMeta>>(emptyList())
    val dashboards: StateFlow<List<DashboardMeta>> = _dashboards
    private val _listLoading = MutableStateFlow(false)
    val listLoading: StateFlow<Boolean> = _listLoading
    private val _listError = MutableStateFlow<String?>(null)
    val listError: StateFlow<String?> = _listError

    private val _dashboard = MutableStateFlow<Dashboard?>(null)
    val dashboard: StateFlow<Dashboard?> = _dashboard
    private val _detailLoading = MutableStateFlow(false)
    val detailLoading: StateFlow<Boolean> = _detailLoading
    private val _detailError = MutableStateFlow<String?>(null)
    val detailError: StateFlow<String?> = _detailError

    private val _panelData = MutableStateFlow<Map<Int, PanelState>>(emptyMap())
    val panelData: StateFlow<Map<Int, PanelState>> = _panelData

    private val _varOptions = MutableStateFlow<Map<String, List<String>>>(emptyMap())
    val varOptions: StateFlow<Map<String, List<String>>> = _varOptions
    private val _selectedVars = MutableStateFlow<Map<String, String>>(emptyMap())
    val selectedVars: StateFlow<Map<String, String>> = _selectedVars

    private var lastFrom = 0L
    private var lastTo = 0L

    fun loadList() {
        if (!ApiClient.isInitialized()) { _listError.value = "API 未初始化，请先登录"; return }
        if (_listLoading.value) return
        _listLoading.value = true
        _listError.value = null
        viewModelScope.launch {
            try {
                _dashboards.value = ApiClient.api.dashboards().dashboards.orEmpty().sortedByDescending { it.updated_at }
            } catch (e: CancellationException) { throw e }
            catch (e: Exception) { _listError.value = e.message ?: "加载失败" }
            finally { _listLoading.value = false }
        }
    }

    /** 加载看板并查询其全部面板（from/to 为 Unix 秒）。 */
    fun loadDashboard(id: String, from: Long, to: Long) {
        if (!ApiClient.isInitialized()) { _detailError.value = "API 未初始化"; return }
        lastFrom = from; lastTo = to
        _detailLoading.value = true
        _detailError.value = null
        _panelData.value = emptyMap()
        viewModelScope.launch {
            try {
                val d = ApiClient.api.dashboard(id)
                _dashboard.value = d
                initVars(d)
                queryAll(from, to)
            } catch (e: CancellationException) { throw e }
            catch (e: Exception) { _detailError.value = e.message ?: "加载失败" }
            finally { _detailLoading.value = false }
        }
    }

    /** 解析模板变量的候选值与默认选中；query 型变量异步向服务端取值。 */
    private fun initVars(d: Dashboard) {
        val sel = mutableMapOf<String, String>()
        val opts = mutableMapOf<String, List<String>>()
        d.vars.orEmpty().forEach { v ->
            when (v.type) {
                "custom" -> {
                    val base = v.options.orEmpty()
                    opts[v.name] = if (v.include_all) listOf("\$__all") + base else base
                    sel[v.name] = v.current.ifBlank {
                        if (v.include_all) "\$__all" else base.firstOrNull().orEmpty()
                    }
                }
                "query" -> {
                    sel[v.name] = v.current.ifBlank { if (v.include_all) "\$__all" else "" }
                }
                else -> { sel[v.name] = v.current }
            }
        }
        _selectedVars.value = sel
        _varOptions.value = opts
        d.vars.orEmpty().filter { it.type == "query" }.forEach { v ->
            viewModelScope.launch {
                try {
                    val values = ApiClient.api.dashboardVarValues(
                        VarValuesReq(
                            name = v.name, label = v.label, type = v.type, query = v.query,
                            options = v.options, current = v.current, multi = v.multi,
                            include_all = v.include_all, datasource = d.datasource
                        )
                    ).values.orEmpty()
                    val withAll = if (v.include_all) listOf("\$__all") + values else values
                    _varOptions.value = _varOptions.value + (v.name to withAll)
                    val cur = _selectedVars.value[v.name]
                    if (cur.isNullOrBlank() && withAll.isNotEmpty()) {
                        _selectedVars.value = _selectedVars.value + (v.name to withAll.first())
                        queryAll(lastFrom, lastTo)
                    }
                } catch (e: CancellationException) { throw e } catch (_: Exception) {}
            }
        }
    }

    fun setVar(name: String, value: String) {
        if (_selectedVars.value[name] == value) return
        _selectedVars.value = _selectedVars.value + (name to value)
        queryAll(lastFrom, lastTo)
    }

    fun queryAll(from: Long, to: Long) {
        lastFrom = from; lastTo = to
        val d = _dashboard.value ?: return
        val vars = _selectedVars.value
        d.panels.orEmpty().forEach { panel -> queryPanel(panel, d.datasource, vars, from, to) }
    }

    private fun queryPanel(panel: DashPanel, dashDs: String, vars: Map<String, String>, from: Long, to: Long) {
        val ds = panel.datasource.ifBlank { dashDs }
        val targets = panel.targets.orEmpty().filter { it.expr.isNotBlank() }
        val typ = panel.type.trim().lowercase()
        when (typ) {
            "text", "unsupported" -> setPanel(panel.id, PanelState.None)
            "alertlist" -> {
                setPanel(panel.id, PanelState.Loading)
                viewModelScope.launch {
                    try {
                        var alerts = ApiClient.api.alerts()
                        val kw = targets.firstOrNull()?.expr?.trim()?.lowercase().orEmpty()
                        if (kw.isNotBlank()) {
                            alerts = alerts.filter { a ->
                                listOf(a.message, a.type, a.hostname, a.level, a.host_id)
                                    .joinToString(" ").lowercase().contains(kw)
                            }
                        }
                        val rank = mapOf("critical" to 0, "warning" to 1, "info" to 2)
                        alerts = alerts.sortedBy { rank[it.level.lowercase()] ?: 3 }.take(200)
                        setPanel(panel.id, PanelState.Alerts(alerts))
                    } catch (e: CancellationException) { throw e }
                    catch (e: Exception) { setPanel(panel.id, PanelState.Err(e.message ?: "加载告警失败")) }
                }
            }
            "logs" -> {
                if (targets.isEmpty()) { setPanel(panel.id, PanelState.Err("未配置 LogQL 查询")); return }
                setPanel(panel.id, PanelState.Loading)
                viewModelScope.launch {
                    try {
                        val all = mutableListOf<DashLogLine>()
                        var available = true
                        var err: String? = null
                        for (t in targets) {
                            val r = ApiClient.api.dashboardQueryLogs(
                                PanelQueryReq(expr = t.expr, from = from, to = to, vars = vars, datasource = ds, limit = 200)
                            )
                            if (!r.available) available = false
                            if (!r.error.isNullOrBlank()) err = r.error
                            all += r.lines.orEmpty()
                        }
                        when {
                            !available -> setPanel(panel.id, PanelState.Err("该面板需选择一个 Loki 数据源"))
                            err != null && all.isEmpty() -> setPanel(panel.id, PanelState.Err(err))
                            else -> setPanel(panel.id, PanelState.Logs(all.sortedByDescending { it.ts_ms }.take(200), available))
                        }
                    } catch (e: CancellationException) { throw e }
                    catch (e: Exception) { setPanel(panel.id, PanelState.Err(e.message ?: "日志查询失败")) }
                }
            }
            // 区间查询：趋势 / 状态色带 / 热力图 / Stat（末点 + sparkline）
            "timeseries", "graph", "state-timeline", "statetimeline", "heatmap", "stat" -> {
                if (targets.isEmpty()) { setPanel(panel.id, PanelState.Err("未配置查询")); return }
                setPanel(panel.id, PanelState.Loading)
                viewModelScope.launch {
                    try {
                        val all = mutableListOf<DashSeries>()
                        for (t in targets) {
                            val r = ApiClient.api.dashboardQuery(
                                PanelQueryReq(expr = t.expr, from = from, to = to, vars = vars, datasource = ds)
                            )
                            if (r.available == false) {
                                setPanel(panel.id, PanelState.Err(r.error ?: "数据源不可用"))
                                return@launch
                            }
                            r.series.orEmpty().forEach { all += DashSeries(it.labels, it.points, t.legend) }
                        }
                        setPanel(panel.id, PanelState.Range(all))
                    } catch (e: CancellationException) { throw e }
                    catch (e: Exception) { setPanel(panel.id, PanelState.Err(e.message ?: "查询失败")) }
                }
            }
            // 即时查询：仪表 / 条形 / 饼图 / 柱状 / 直方图 / 表格
            "gauge", "bargauge", "table", "piechart", "pie", "barchart", "bar", "histogram" -> {
                if (targets.isEmpty()) { setPanel(panel.id, PanelState.Err("未配置查询")); return }
                setPanel(panel.id, PanelState.Loading)
                viewModelScope.launch {
                    try {
                        val all = mutableListOf<DashValue>()
                        for (t in targets) {
                            val r = ApiClient.api.dashboardQueryInstant(
                                PanelQueryReq(expr = t.expr, from = from, to = to, vars = vars, datasource = ds)
                            )
                            if (r.available == false) {
                                setPanel(panel.id, PanelState.Err(r.error ?: "数据源不可用"))
                                return@launch
                            }
                            r.series.orEmpty().forEach { all += DashValue(it.labels, it.value, t.legend) }
                        }
                        setPanel(panel.id, PanelState.Instant(all))
                    } catch (e: CancellationException) { throw e }
                    catch (e: Exception) { setPanel(panel.id, PanelState.Err(e.message ?: "查询失败")) }
                }
            }
            else -> setPanel(panel.id, PanelState.None)
        }
    }

    private fun setPanel(id: Int, state: PanelState) {
        _panelData.value = _panelData.value + (id to state)
    }

    fun clearDetail() {
        _dashboard.value = null
        _panelData.value = emptyMap()
        _varOptions.value = emptyMap()
        _selectedVars.value = emptyMap()
    }
}
