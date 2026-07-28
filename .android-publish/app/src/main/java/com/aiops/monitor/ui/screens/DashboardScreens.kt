package com.aiops.monitor.ui.screens

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectHorizontalDragGestures
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.gestures.detectTransformGestures
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Fullscreen
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.window.DialogProperties
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.DrawScope
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavHostController
import com.aiops.monitor.data.models.Alert
import com.aiops.monitor.data.models.DashLogLine
import com.aiops.monitor.data.models.DashPanel
import com.aiops.monitor.data.models.DashVar
import com.aiops.monitor.data.models.DashboardMeta
import com.aiops.monitor.ui.Routes
import com.aiops.monitor.ui.components.*
import com.aiops.monitor.ui.viewmodel.DashSeries
import com.aiops.monitor.ui.viewmodel.DashValue
import com.aiops.monitor.ui.viewmodel.DashboardViewModel
import com.aiops.monitor.ui.viewmodel.PanelState
import kotlin.math.max
import kotlin.math.min
import kotlin.math.roundToInt
import kotlin.math.sqrt

private val DashBlue = Color(0xFF4F7FFF)
private val seriesPalette = listOf(
    Color(0xFF4F7FFF), Color(0xFF00A86B), Color(0xFFF59E0B), Color(0xFFEF4444),
    Color(0xFF8B5CF6), Color(0xFF06B6D4), Color(0xFFEC4899), Color(0xFF14B8A6)
)

/* ───────────────────────── 列表 ───────────────────────── */

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DashboardListScreen(navController: NavHostController, modifier: Modifier = Modifier) {
    val vm: DashboardViewModel = viewModel()
    val dashboards by vm.dashboards.collectAsState()
    val loading by vm.listLoading.collectAsState()
    val error by vm.listError.collectAsState()

    LaunchedEffect(Unit) { vm.loadList() }

    Scaffold(
        modifier = modifier,
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            TopAppBar(
                title = {
                    Column {
                        Text("仪表盘", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface)
                        Text("自定义面板 · 只读查看", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                },
                navigationIcon = { IconButton(onClick = { navController.popBackStack() }) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回", tint = MaterialTheme.colorScheme.onSurface) } },
                actions = { IconButton(onClick = { vm.loadList() }) { Icon(Icons.Default.Refresh, "刷新", tint = MaterialTheme.colorScheme.onSurface) } },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background)
            )
        }
    ) { padding ->
        when {
            loading && dashboards.isEmpty() -> LoadingBox(Modifier.fillMaxSize().padding(padding))
            error != null && dashboards.isEmpty() -> StateBox("加载失败：$error", Modifier.fillMaxSize().padding(padding), onRetry = vm::loadList)
            dashboards.isEmpty() -> StateBox("暂无仪表盘\n可在 Web 端创建自定义看板或导入 Grafana 看板", Modifier.fillMaxSize().padding(padding))
            else -> LazyColumn(
                Modifier.fillMaxSize().padding(padding),
                contentPadding = PaddingValues(16.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                items(dashboards, key = { it.id }) { d -> DashboardMetaCard(d) { navController.navigate(Routes.dashboardView(d.id)) } }
            }
        }
    }
}

@Composable
private fun DashboardMetaCard(d: DashboardMeta, onClick: () -> Unit) {
    Card(
        Modifier.fillMaxWidth().clickable(onClick = onClick),
        shape = RoundedCornerShape(14.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 1.dp)
    ) {
        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(d.name.ifBlank { "(未命名)" }, fontWeight = FontWeight.Bold, fontSize = 15.sp, color = MaterialTheme.colorScheme.onSurface, modifier = Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis)
                sourceBadge(d.source)?.let { (label, color) -> StatusPill(label, color) }
            }
            if (d.description.isNotBlank()) Text(d.description, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 2, overflow = TextOverflow.Ellipsis)
            Text("${d.panels} 个面板", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

private fun sourceBadge(source: String): Pair<String, Color>? = when {
    source.startsWith("grafana") -> "Grafana" to Color(0xFFF59E0B)
    source == "import" -> "导入" to Color(0xFF06B6D4)
    source == "ai" -> "AI" to Color(0xFF8B5CF6)
    else -> null
}

/* ───────────────────────── 查看 ───────────────────────── */

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DashboardViewScreen(dashboardId: String, navController: NavHostController, modifier: Modifier = Modifier) {
    val vm: DashboardViewModel = viewModel()
    val dashboard by vm.dashboard.collectAsState()
    val panelData by vm.panelData.collectAsState()
    val varOptions by vm.varOptions.collectAsState()
    val selectedVars by vm.selectedVars.collectAsState()
    val loading by vm.detailLoading.collectAsState()
    val error by vm.detailError.collectAsState()

    var range by remember { mutableStateOf("1h") }
    fun fromTo(): Pair<Long, Long> {
        val to = System.currentTimeMillis() / 1000
        val sec = when (range) { "6h" -> 21600L; "24h" -> 86400L; "3d" -> 259200L; "7d" -> 604800L; else -> 3600L }
        return (to - sec) to to
    }

    LaunchedEffect(dashboardId) { val (f, t) = fromTo(); vm.loadDashboard(dashboardId, f, t) }
    LaunchedEffect(range) { if (dashboard != null) { val (f, t) = fromTo(); vm.queryAll(f, t) } }

    Scaffold(
        modifier = modifier,
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            TopAppBar(
                title = {
                    Column {
                        Text(dashboard?.name ?: "仪表盘", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        val n = dashboard?.panels?.size ?: 0
                        Text("$n 个面板 · 只读", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                },
                navigationIcon = { IconButton(onClick = { navController.popBackStack() }) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回", tint = MaterialTheme.colorScheme.onSurface) } },
                actions = { IconButton(onClick = { val (f, t) = fromTo(); vm.queryAll(f, t) }) { Icon(Icons.Default.Refresh, "刷新", tint = MaterialTheme.colorScheme.onSurface) } },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background)
            )
        }
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding)) {
            Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 6.dp), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                listOf("1h" to "1时", "6h" to "6时", "24h" to "24时", "3d" to "3天", "7d" to "7天").forEach { (k, label) ->
                    FilterChip(selected = range == k, onClick = { range = k }, label = { Text(label, fontSize = 12.sp) })
                }
            }
            // 模板变量选择器（如主机、实例等；无变量时不显示）
            val vars = dashboard?.vars.orEmpty().filter { it.type != "constant" }
            if (vars.isNotEmpty()) {
                Row(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = 16.dp, vertical = 2.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    vars.forEach { v ->
                        VarSelector(v, varOptions[v.name].orEmpty(), selectedVars[v.name] ?: v.current) { value -> vm.setVar(v.name, value) }
                    }
                }
            }
            when {
                loading && dashboard == null -> LoadingBox(Modifier.fillMaxSize())
                error != null && dashboard == null -> StateBox("加载失败：$error", Modifier.fillMaxSize())
                else -> {
                    val panels = remember(dashboard) { dashboard?.panels.orEmpty().sortedWith(compareBy({ it.grid.y }, { it.grid.x })) }
                    LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                        items(panels, key = { it.id }) { p -> PanelCard(p, panelData[p.id] ?: PanelState.Loading) }
                    }
                }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun VarSelector(v: DashVar, options: List<String>, selected: String, onSelect: (String) -> Unit) {
    var open by remember { mutableStateOf(false) }
    fun displayOpt(o: String) = if (o == "\$__all" || o.equals("All", true)) "全部" else o
    Box {
        OutlinedButton(onClick = { if (options.isNotEmpty()) open = true }, contentPadding = PaddingValues(horizontal = 12.dp, vertical = 4.dp)) {
            Text("${dashVarDisplayLabel(v)}: ${displayOpt(selected).ifBlank { "全部" }}", fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Icon(Icons.Default.ArrowDropDown, null, Modifier.size(18.dp))
        }
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            options.forEach { opt ->
                DropdownMenuItem(text = { Text(displayOpt(opt), fontSize = 13.sp) }, onClick = { onSelect(opt); open = false })
            }
            if (options.isEmpty()) DropdownMenuItem(text = { Text("无候选值", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant) }, onClick = { open = false }, enabled = false)
        }
    }
}

@Composable
private fun PanelCard(panel: DashPanel, state: PanelState) {
    Card(
        Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(14.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 1.dp)
    ) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            if (panel.title.isNotBlank()) Text(panel.title, fontWeight = FontWeight.Bold, fontSize = 14.sp, color = MaterialTheme.colorScheme.onSurface)
            when (panel.type.trim().lowercase()) {
                "text" -> Text(panel.text.ifBlank { "(空)" }, fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                "unsupported" -> PanelHint("⚠ 暂不支持的面板类型：${panel.raw_type.ifBlank { panel.type }.ifBlank { "未知" }}")
                "timeseries", "graph" -> PanelBody(state) { s ->
                    if (s is PanelState.Range) {
                        var showFull by remember(s) { mutableStateOf(false) }
                        MultiSeriesChart(s.series, panel.unit, 280.dp, onExpand = { showFull = true })
                        if (showFull) FullscreenDashChartDialog(panel.title, s.series, panel.unit) { showFull = false }
                    } else PanelHint("无数据")
                }
                "stat" -> PanelBody(state) { s ->
                    when (s) {
                        is PanelState.Range -> StatRangePanel(s.series, panel)
                        is PanelState.Instant -> StatPanel(s.values, panel)
                        else -> PanelHint("无数据")
                    }
                }
                "gauge" -> PanelBody(state) { s -> if (s is PanelState.Instant) RadialGaugePanel(s.values, panel) else PanelHint("无数据") }
                "bargauge" -> PanelBody(state) { s -> if (s is PanelState.Instant) BarGaugePanel(s.values, panel) else PanelHint("无数据") }
                "piechart", "pie" -> PanelBody(state) { s -> if (s is PanelState.Instant) PieDonutPanel(s.values, panel) else PanelHint("无数据") }
                "barchart", "bar" -> PanelBody(state) { s -> if (s is PanelState.Instant) VerticalBarPanel(s.values, panel) else PanelHint("无数据") }
                "histogram" -> PanelBody(state) { s -> if (s is PanelState.Instant) HistogramPanel(s.values, panel) else PanelHint("无数据") }
                "table" -> PanelBody(state) { s -> if (s is PanelState.Instant) TablePanel(s.values, panel) else PanelHint("无数据") }
                "state-timeline", "statetimeline" -> PanelBody(state) { s ->
                    if (s is PanelState.Range) StateTimelinePanel(s.series, panel) else PanelHint("无数据")
                }
                "heatmap" -> PanelBody(state) { s ->
                    if (s is PanelState.Range) HeatmapPanel(s.series, panel) else PanelHint("无数据")
                }
                "alertlist" -> PanelBody(state) { s ->
                    if (s is PanelState.Alerts) AlertListPanel(s.items) else PanelHint("无数据")
                }
                "logs" -> PanelBody(state) { s ->
                    if (s is PanelState.Logs) LogsPanel(s.lines) else PanelHint("无数据")
                }
                else -> PanelHint("移动端暂不支持「${panel.raw_type.ifBlank { panel.type }.ifBlank { "该类型" }}」面板")
            }
        }
    }
}

@Composable
private fun PanelBody(state: PanelState, content: @Composable (PanelState) -> Unit) {
    when (state) {
        is PanelState.Loading -> Box(Modifier.fillMaxWidth().height(80.dp), contentAlignment = Alignment.Center) { CircularProgressIndicator(Modifier.size(22.dp), strokeWidth = 2.dp, color = DashBlue) }
        is PanelState.Err -> PanelHint("查询失败：${state.msg}")
        is PanelState.None -> PanelHint("无数据")
        else -> content(state)
    }
}

@Composable
private fun PanelHint(text: String) {
    Text(text, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(vertical = 8.dp))
}

/* ── timeseries 多序列折线（内联：拖动查看 + 放大按钮） ── */
@Composable
private fun MultiSeriesChart(series: List<DashSeries>, unit: String, height: Dp, onExpand: () -> Unit) {
    val data = remember(series) { series.map { it.points.orEmpty().filter { p -> p.size >= 2 } } }
    if (data.all { it.isEmpty() }) { PanelHint("无数据"); return }
    val legends = remember(series) { dashLegends(series.map { it.labels to it.legendFmt }) }
    val bounds = remember(data) { chartBounds(data) }
    val gridColor = MaterialTheme.colorScheme.outlineVariant
    var cursorFrac by remember { mutableStateOf<Float?>(null) }

    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            val readout = cursorFrac?.let { inspectText(it, data, legends, bounds, 0f, 1f, unit) }
            Text(readout ?: "↑ ${fmtUnit(bounds.maxY, unit, -1)}   ↓ ${fmtUnit(bounds.minY, unit, -1)}", fontSize = 9.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis)
            IconButton(onClick = onExpand, modifier = Modifier.size(28.dp)) { Icon(Icons.Default.Fullscreen, "放大", tint = DashBlue, modifier = Modifier.size(18.dp)) }
        }
        Canvas(
            Modifier.fillMaxWidth().height(height).pointerInput(data) {
                detectHorizontalDragGestures(
                    onDragStart = { cursorFrac = (it.x / size.width).coerceIn(0f, 1f) },
                    onDragEnd = { cursorFrac = null },
                    onDragCancel = { cursorFrac = null },
                    onHorizontalDrag = { change, _ -> cursorFrac = (change.position.x / size.width).coerceIn(0f, 1f) }
                )
            }
        ) {
            drawLine(gridColor, Offset(0f, size.height - 1), Offset(size.width, size.height - 1), 1f)
            drawSeries(data, bounds, 0f, 1f)
            cursorFrac?.let { f -> drawLine(DashBlue.copy(alpha = 0.6f), Offset(f * size.width, 0f), Offset(f * size.width, size.height), 1.5f) }
        }
        if (series.size > 1) LegendFlow(series.size, legends)
    }
}

/* ── 放大查看：双指缩放 / 拖动平移 / 点按查看 / 双击还原 ── */
@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
private fun FullscreenDashChartDialog(title: String, series: List<DashSeries>, unit: String, onDismiss: () -> Unit) {
    val data = remember(series) { series.map { it.points.orEmpty().filter { p -> p.size >= 2 } } }
    val legends = remember(series) { dashLegends(series.map { it.labels to it.legendFmt }) }
    val bounds = remember(data) { chartBounds(data) }
    val gridColor = MaterialTheme.colorScheme.outlineVariant
    var vStart by remember { mutableFloatStateOf(0f) }
    var vEnd by remember { mutableFloatStateOf(1f) }
    var cursorFrac by remember { mutableStateOf<Float?>(null) }

    AlertDialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false),
        modifier = Modifier.fillMaxSize(),
        confirmButton = {},
        title = null,
        text = {
            Column(Modifier.fillMaxSize(), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Row(Modifier.fillMaxWidth(), Arrangement.SpaceBetween, Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        Text(title.ifBlank { "图表" }, fontWeight = FontWeight.Bold, fontSize = 16.sp, color = MaterialTheme.colorScheme.onSurface, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Text("双指缩放 · 拖动平移 · 点按查看 · 双击还原", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    if (vStart > 0f || vEnd < 1f) TextButton(onClick = { vStart = 0f; vEnd = 1f; cursorFrac = null }) { Text("还原") }
                    IconButton(onClick = onDismiss) { Icon(Icons.Default.Close, "关闭", tint = MaterialTheme.colorScheme.onSurfaceVariant) }
                }
                Text(cursorFrac?.let { inspectText(it, data, legends, bounds, vStart, vEnd, unit) } ?: "↑ ${fmtUnit(bounds.maxY, unit, -1)}   ↓ ${fmtUnit(bounds.minY, unit, -1)}",
                    fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 2, overflow = TextOverflow.Ellipsis)
                Canvas(
                    Modifier.fillMaxWidth().weight(1f)
                        .pointerInput(Unit) { detectTapGestures(onDoubleTap = { vStart = 0f; vEnd = 1f; cursorFrac = null }, onTap = { cursorFrac = (it.x / size.width).coerceIn(0f, 1f) }) }
                        .pointerInput(Unit) {
                            detectTransformGestures { centroid, pan, zoom, _ ->
                                val w = size.width.toFloat().coerceAtLeast(1f)
                                val span = (vEnd - vStart)
                                val cf = vStart + (centroid.x / w) * span
                                val newSpan = (span / zoom).coerceIn(0.02f, 1f)
                                var ns = (cf - (centroid.x / w) * newSpan) - (pan.x / w) * newSpan
                                var ne = ns + newSpan
                                if (ns < 0f) { ns = 0f; ne = newSpan }
                                if (ne > 1f) { ne = 1f; ns = 1f - newSpan }
                                vStart = ns.coerceIn(0f, 1f); vEnd = ne.coerceIn(0f, 1f)
                            }
                        }
                ) {
                    drawLine(gridColor, Offset(0f, size.height - 1), Offset(size.width, size.height - 1), 1f)
                    drawSeries(data, bounds, vStart, vEnd)
                    cursorFrac?.let { f -> drawLine(DashBlue.copy(alpha = 0.7f), Offset(f * size.width, 0f), Offset(f * size.width, size.height), 1.6f) }
                }
                if (series.size > 1) LegendFlow(series.size, legends)
            }
        },
        containerColor = MaterialTheme.colorScheme.surface
    )
}

/* ── 图例：横向紧凑（左右排列、自动换行） ── */
@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun LegendFlow(count: Int, legends: List<String>) {
    FlowRow(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(10.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) {
        (0 until minOf(count, 16)).forEach { i ->
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                Box(Modifier.size(8.dp).clip(RoundedCornerShape(2.dp)).background(seriesPalette[i % seriesPalette.size]))
                Text(legends.getOrElse(i) { "series" }, fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
        }
    }
}

private data class ChartBounds(val minX: Double, val maxX: Double, val minY: Double, val maxY: Double)

private fun chartBounds(data: List<List<List<Double>>>): ChartBounds {
    var minX = Double.MAX_VALUE; var maxX = -Double.MAX_VALUE; var minY = Double.MAX_VALUE; var maxY = -Double.MAX_VALUE
    data.forEach { pts -> pts.forEach { p -> if (p[0] < minX) minX = p[0]; if (p[0] > maxX) maxX = p[0]; if (p[1] < minY) minY = p[1]; if (p[1] > maxY) maxY = p[1] } }
    if (minX > maxX) { minX = 0.0; maxX = 1.0 }
    if (minY > maxY) { minY = 0.0; maxY = 1.0 }
    return ChartBounds(minX, maxX, minY, maxY)
}

private fun DrawScope.drawSeries(data: List<List<List<Double>>>, b: ChartBounds, vStart: Float, vEnd: Float) {
    val w = size.width; val h = size.height
    val spanX = (b.maxX - b.minX).takeIf { it > 0 } ?: 1.0
    val spanY = (b.maxY - b.minY).takeIf { it > 0 } ?: 1.0
    val visMinX = b.minX + vStart * spanX
    val visSpan = ((vEnd - vStart) * spanX).takeIf { it > 0 } ?: 1.0
    data.forEachIndexed { i, pts ->
        if (pts.isEmpty()) return@forEachIndexed
        val color = seriesPalette[i % seriesPalette.size]
        val path = Path()
        pts.forEachIndexed { j, p ->
            val px = (((p[0] - visMinX) / visSpan).toFloat()) * w
            val py = h - (((p[1] - b.minY) / spanY).toFloat()) * h
            if (j == 0) path.moveTo(px, py) else path.lineTo(px, py)
        }
        drawPath(path, color, style = Stroke(width = 2.2f))
    }
}

private fun inspectText(frac: Float, data: List<List<List<Double>>>, legends: List<String>, b: ChartBounds, vStart: Float, vEnd: Float, unit: String): String {
    val spanX = (b.maxX - b.minX).takeIf { it > 0 } ?: 1.0
    val x = b.minX + (vStart + frac * (vEnd - vStart)) * spanX
    val time = java.text.SimpleDateFormat("MM-dd HH:mm", java.util.Locale.getDefault()).format(java.util.Date(x.toLong() * 1000))
    val parts = data.mapIndexedNotNull { i, pts ->
        val p = pts.minByOrNull { kotlin.math.abs(it[0] - x) } ?: return@mapIndexedNotNull null
        "${legends.getOrElse(i) { "s${i + 1}" }} ${fmtUnit(p[1], unit, -1)}"
    }.take(6)
    return "$time · ${parts.joinToString("  ")}"
}

/* ── stat：大数字 + 阈值色 + sparkline（区间末点，与 Web 一致） ── */
@Composable
private fun StatRangePanel(series: List<DashSeries>, panel: DashPanel) {
    if (series.isEmpty()) { PanelHint("无数据"); return }
    val s0 = series.first()
    val pts = s0.points.orEmpty().filter { it.size >= 2 }
    if (pts.isEmpty()) { PanelHint("无数据"); return }
    val val0 = pts.last()[1]
    val color = thresholdColor(val0, panel)
    val legends = remember(series) { dashLegends(series.map { it.labels to it.legendFmt }) }
    Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.fillMaxWidth()) {
        Text(fmtUnit(val0, panel.unit, panel.decimals), fontWeight = FontWeight.Black, fontSize = 30.sp, color = color, maxLines = 1)
        if (legends.firstOrNull().orEmpty().isNotBlank()) {
            Text(legends.first(), fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
        if (pts.size > 1) {
            val sparkVals = pts.map { it[1] }
            Canvas(Modifier.fillMaxWidth().height(36.dp).padding(top = 4.dp)) {
                drawSparkline(sparkVals, color)
            }
        }
    }
}

@Composable
private fun StatPanel(values: List<DashValue>, panel: DashPanel) {
    if (values.isEmpty()) { PanelHint("无数据"); return }
    val legends = remember(values) { dashLegends(values.map { it.labels to it.legendFmt }) }
    val color = thresholdColor(values.first().value, panel)
    Text(fmtUnit(values.first().value, panel.unit, panel.decimals), fontWeight = FontWeight.Black, fontSize = 30.sp, color = color, maxLines = 1)
    Column(verticalArrangement = Arrangement.spacedBy(3.dp), modifier = Modifier.padding(top = 2.dp)) {
        values.indices.take(12).forEach { i ->
            Row(Modifier.fillMaxWidth(), Arrangement.SpaceBetween, Alignment.CenterVertically) {
                Text(legends.getOrElse(i) { "" }, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
                Text(fmtUnit(values[i].value, panel.unit, panel.decimals), fontSize = 11.sp, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface)
            }
        }
    }
}

private fun DrawScope.drawSparkline(vals: List<Double>, color: Color) {
    if (vals.size < 2) return
    val mn = vals.minOrNull() ?: 0.0
    val mx = vals.maxOrNull() ?: 1.0
    val rng = (mx - mn).takeIf { it > 0 } ?: 1.0
    val w = size.width; val h = size.height
    val path = Path()
    val fill = Path()
    vals.forEachIndexed { i, v ->
        val x = i.toFloat() / (vals.size - 1) * w
        val y = h - ((v - mn) / rng).toFloat() * h
        if (i == 0) { path.moveTo(x, y); fill.moveTo(x, h); fill.lineTo(x, y) }
        else { path.lineTo(x, y); fill.lineTo(x, y) }
    }
    fill.lineTo(w, h); fill.close()
    drawPath(fill, color.copy(alpha = 0.12f))
    drawPath(path, color, style = Stroke(width = 2f, cap = StrokeCap.Round))
}

/* ── gauge：圆环径向仪表（与 Web svgGauge 对齐） ── */
@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun RadialGaugePanel(values: List<DashValue>, panel: DashPanel) {
    if (values.isEmpty()) { PanelHint("无数据"); return }
    val legends = remember(values) { dashLegends(values.map { it.labels to it.legendFmt }) }
    val minV = panel.min ?: 0.0
    val maxV = panel.max ?: when (panel.unit) {
        "percent" -> 100.0
        "percentunit" -> 1.0
        else -> max(values.maxOfOrNull { it.value } ?: 100.0, 1.0)
    }
    val track = MaterialTheme.colorScheme.outlineVariant
    FlowRow(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        values.indices.take(9).forEach { i ->
            val value = values[i].value
            val frac = ((value - minV) / (maxV - minV).coerceAtLeast(1e-9)).toFloat().coerceIn(0f, 1f)
            val color = thresholdColor(value, panel, minV, maxV)
            Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.width(108.dp)) {
                Canvas(Modifier.size(96.dp)) {
                    val stroke = 10.dp.toPx()
                    val r = (min(size.width, size.height) - stroke) / 2f
                    val arcSize = Size(r * 2, r * 2)
                    val topLeft = Offset((size.width - arcSize.width) / 2f, (size.height - arcSize.height) / 2f)
                    drawArc(track, -90f, 360f, false, topLeft = topLeft, size = arcSize, style = Stroke(stroke, cap = StrokeCap.Round))
                    drawArc(color, -90f, frac * 360f, false, topLeft = topLeft, size = arcSize, style = Stroke(stroke, cap = StrokeCap.Round))
                }
                Text(fmtUnit(value, panel.unit, panel.decimals), fontWeight = FontWeight.Bold, fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurface)
                Text(legends.getOrElse(i) { "" }.ifBlank { panel.title }, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
        }
    }
}

/* ── bargauge：横向进度条 ── */
@Composable
private fun BarGaugePanel(values: List<DashValue>, panel: DashPanel) {
    if (values.isEmpty()) { PanelHint("无数据"); return }
    val legends = remember(values) { dashLegends(values.map { it.labels to it.legendFmt }) }
    val minV = panel.min ?: 0.0
    val maxV = panel.max ?: (if (panel.unit == "percent") 100.0 else max(values.maxOfOrNull { it.value } ?: 100.0, 1.0))
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        values.indices.take(16).forEach { i ->
            val value = values[i].value
            val frac = ((value - minV) / (maxV - minV).coerceAtLeast(1e-9)).toFloat().coerceIn(0f, 1f)
            val color = thresholdColor(value, panel, minV, maxV)
            Row(Modifier.fillMaxWidth(), Arrangement.SpaceBetween) {
                Text(legends.getOrElse(i) { "" }.ifBlank { panel.title }, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, modifier = Modifier.weight(1f))
                Text(fmtUnit(value, panel.unit, panel.decimals), fontSize = 12.sp, fontWeight = FontWeight.Bold, color = color)
            }
            Box(Modifier.fillMaxWidth().height(8.dp).clip(RoundedCornerShape(4.dp)).background(MaterialTheme.colorScheme.surfaceVariant)) {
                Box(Modifier.fillMaxWidth(frac).fillMaxHeight().clip(RoundedCornerShape(4.dp)).background(color))
            }
        }
    }
}

/* ── pie：环形图 + 图例占比 ── */
@Composable
private fun PieDonutPanel(values: List<DashValue>, panel: DashPanel) {
    if (values.isEmpty()) { PanelHint("无数据"); return }
    val legends = remember(values) { dashLegends(values.map { it.labels to it.legendFmt }) }
    val items = values.indices.map { i -> Triple(legends.getOrElse(i) { "系列${i + 1}" }, values[i].value.coerceAtLeast(0.0), seriesPalette[i % seriesPalette.size]) }
        .filter { it.second > 0 }
        .sortedByDescending { it.second }
        .take(12)
    if (items.isEmpty()) { PanelHint("无数据"); return }
    val total = items.sumOf { it.second }.coerceAtLeast(1e-9)
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.CenterVertically) {
        Box(Modifier.size(140.dp), contentAlignment = Alignment.Center) {
            Canvas(Modifier.fillMaxSize()) {
                val stroke = 22.dp.toPx()
                val r = (min(size.width, size.height) - stroke) / 2f
                val arcSize = Size(r * 2, r * 2)
                val topLeft = Offset((size.width - arcSize.width) / 2f, (size.height - arcSize.height) / 2f)
                var start = -90f
                items.forEach { (_, v, c) ->
                    val sweep = (v / total * 360.0).toFloat()
                    drawArc(c, start, sweep, false, topLeft = topLeft, size = arcSize, style = Stroke(stroke, cap = StrokeCap.Butt))
                    start += sweep
                }
            }
            Text(fmtUnit(total, panel.unit, panel.decimals), fontWeight = FontWeight.Bold, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurface, maxLines = 1)
        }
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            items.forEach { (name, v, c) ->
                val pct = v / total * 100
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    Box(Modifier.size(8.dp).clip(RoundedCornerShape(2.dp)).background(c))
                    Text(name, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
                    Text(fmtUnit(v, panel.unit, panel.decimals), fontSize = 11.sp, fontWeight = FontWeight.SemiBold)
                    Text(if (pct >= 10) "%.0f%%".format(pct) else "%.1f%%".format(pct), fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
        }
    }
}

/* ── barchart：纵向柱状图 ── */
@Composable
private fun VerticalBarPanel(values: List<DashValue>, panel: DashPanel) {
    if (values.isEmpty()) { PanelHint("无数据"); return }
    val legends = remember(values) { dashLegends(values.map { it.labels to it.legendFmt }) }
    val items = values.indices.map { i -> Triple(legends.getOrElse(i) { "s${i + 1}" }, values[i].value, seriesPalette[i % seriesPalette.size]) }
        .sortedByDescending { it.second }
        .take(16)
    val mx = items.maxOfOrNull { it.second }?.coerceAtLeast(1e-9) ?: 1.0
    Row(
        Modifier.fillMaxWidth().height(180.dp).horizontalScroll(rememberScrollState()),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalAlignment = Alignment.Bottom
    ) {
        items.forEach { (name, v, c) ->
            val hFrac = (v / mx).toFloat().coerceIn(0.02f, 1f)
            Column(
                Modifier.width(44.dp).fillMaxHeight(),
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.Bottom
            ) {
                Text(fmtUnit(v, panel.unit, panel.decimals), fontSize = 9.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Spacer(Modifier.height(4.dp))
                Box(Modifier.fillMaxWidth().weight(1f), contentAlignment = Alignment.BottomCenter) {
                    Box(Modifier.fillMaxWidth(0.72f).fillMaxHeight(hFrac).clip(RoundedCornerShape(topStart = 4.dp, topEnd = 4.dp)).background(c))
                }
                Spacer(Modifier.height(4.dp))
                Text(name, fontSize = 9.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 2, overflow = TextOverflow.Ellipsis)
            }
        }
    }
}

/* ── histogram：分箱分布柱 ── */
@Composable
private fun HistogramPanel(values: List<DashValue>, panel: DashPanel) {
    val vals = values.map { it.value }.filter { it.isFinite() }
    if (vals.isEmpty()) { PanelHint("无数据"); return }
    val mn = vals.minOrNull() ?: 0.0
    val mx = vals.maxOrNull() ?: 1.0
    val bins = min(16, max(4, (sqrt(vals.size.toDouble()).roundToInt() + 2)))
    val w = ((mx - mn) / bins).takeIf { it > 0 } ?: 1.0
    val counts = IntArray(bins)
    vals.forEach { v ->
        var i = ((v - mn) / w).toInt()
        if (i >= bins) i = bins - 1
        if (i < 0) i = 0
        counts[i]++
    }
    val mxc = counts.maxOrNull()?.coerceAtLeast(1) ?: 1
    Row(
        Modifier.fillMaxWidth().height(160.dp).horizontalScroll(rememberScrollState()),
        horizontalArrangement = Arrangement.spacedBy(4.dp),
        verticalAlignment = Alignment.Bottom
    ) {
        counts.forEachIndexed { i, c ->
            val lo = mn + i * w
            val hFrac = (c.toFloat() / mxc).coerceIn(0.02f, 1f)
            Column(Modifier.width(36.dp).fillMaxHeight(), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.Bottom) {
                Text("$c", fontSize = 9.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                Box(Modifier.fillMaxWidth().weight(1f), contentAlignment = Alignment.BottomCenter) {
                    Box(Modifier.fillMaxWidth(0.8f).fillMaxHeight(hFrac).clip(RoundedCornerShape(topStart = 3.dp, topEnd = 3.dp)).background(DashBlue))
                }
                Text(fmtUnit(lo, panel.unit, panel.decimals), fontSize = 8.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
        }
    }
}

/* ── state-timeline：状态色带 ── */
@Composable
private fun StateTimelinePanel(series: List<DashSeries>, panel: DashPanel) {
    if (series.isEmpty()) { PanelHint("无数据"); return }
    val legends = remember(series) { dashLegends(series.map { it.labels to it.legendFmt }) }
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        series.take(16).forEachIndexed { idx, s ->
            val pts = downsamplePts(s.points.orEmpty().filter { it.size >= 2 }, 64)
            if (pts.isEmpty()) return@forEachIndexed
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(legends.getOrElse(idx) { "s${idx + 1}" }, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.width(72.dp))
                Row(Modifier.weight(1f).height(14.dp).clip(RoundedCornerShape(4.dp))) {
                    pts.forEach { p ->
                        Box(Modifier.weight(1f).fillMaxHeight().background(stateColor(p[1], panel)))
                    }
                }
            }
        }
    }
}

/* ── heatmap：时间 × 序列密度色块 ── */
@Composable
private fun HeatmapPanel(series: List<DashSeries>, panel: DashPanel) {
    if (series.isEmpty()) { PanelHint("无数据"); return }
    val rows = series.take(24)
    val legends = remember(rows) { dashLegends(rows.map { it.labels to it.legendFmt }) }
    var mn = Double.POSITIVE_INFINITY
    var mx = Double.NEGATIVE_INFINITY
    rows.forEach { s -> s.points.orEmpty().forEach { p -> if (p.size >= 2) { if (p[1] < mn) mn = p[1]; if (p[1] > mx) mx = p[1] } } }
    if (mn > mx) { PanelHint("无数据"); return }
    val rng = (mx - mn).takeIf { it > 0 } ?: 1.0
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        rows.forEachIndexed { idx, s ->
            val pts = downsamplePts(s.points.orEmpty().filter { it.size >= 2 }, 48)
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                Text(legends.getOrElse(idx) { "s${idx + 1}" }, fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.width(68.dp))
                Row(Modifier.weight(1f).height(12.dp).clip(RoundedCornerShape(2.dp))) {
                    pts.forEach { p ->
                        val t = ((p[1] - mn) / rng).toFloat().coerceIn(0f, 1f)
                        Box(Modifier.weight(1f).fillMaxHeight().background(heatColor(t)))
                    }
                }
            }
        }
    }
}

/* ── alertlist ── */
@Composable
private fun AlertListPanel(items: List<Alert>) {
    if (items.isEmpty()) {
        Text("✓ 当前无告警", fontSize = 13.sp, color = Color(0xFF00A86B), modifier = Modifier.padding(vertical = 8.dp))
        return
    }
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        items.take(50).forEach { a ->
            val c = when (a.level.lowercase()) {
                "critical" -> Color(0xFFEF4444)
                "warning" -> Color(0xFFF59E0B)
                else -> DashBlue
            }
            Row(
                Modifier.fillMaxWidth().clip(RoundedCornerShape(8.dp)).background(c.copy(alpha = 0.08f)).padding(horizontal = 10.dp, vertical = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Box(Modifier.size(8.dp).clip(RoundedCornerShape(50)).background(c))
                Column(Modifier.weight(1f)) {
                    Text(a.message.ifBlank { a.type }, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurface, maxLines = 2, overflow = TextOverflow.Ellipsis)
                    if (a.hostname.isNotBlank()) Text(a.hostname, fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                StatusPill(a.level.ifBlank { "info" }, c)
            }
        }
    }
}

/* ── logs ── */
@Composable
private fun LogsPanel(lines: List<DashLogLine>) {
    if (lines.isEmpty()) { PanelHint("该范围无日志"); return }
    val fmt = remember { java.text.SimpleDateFormat("MM-dd HH:mm:ss", java.util.Locale.getDefault()) }
    Column(
        Modifier.fillMaxWidth().heightIn(max = 320.dp),
        verticalArrangement = Arrangement.spacedBy(4.dp)
    ) {
        lines.take(80).forEach { line ->
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(
                    if (line.ts_ms > 0) fmt.format(java.util.Date(line.ts_ms)) else "",
                    fontSize = 10.sp,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.width(78.dp)
                )
                Text(line.line, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurface, modifier = Modifier.weight(1f))
            }
        }
    }
}

private fun downsamplePts(pts: List<List<Double>>, n: Int): List<List<Double>> {
    if (pts.size <= n) return pts
    val step = pts.size.toDouble() / n
    return (0 until n).map { pts[min(pts.size - 1, (it * step).toInt())] }
}

private fun thresholdColor(v: Double, panel: DashPanel, minV: Double? = null, maxV: Double? = null): Color {
    var pct: Double? = null
    when (panel.unit) {
        "percent" -> pct = v
        "percentunit" -> pct = v * 100
        else -> {
            val lo = minV ?: panel.min
            val hi = maxV ?: panel.max
            if (lo != null && hi != null && hi > lo) pct = (v - lo) / (hi - lo) * 100
        }
    }
    val p = pct ?: return DashBlue
    return when {
        p >= 90 -> Color(0xFFEF4444)
        p >= 75 -> Color(0xFFF59E0B)
        else -> Color(0xFF00A86B)
    }
}

private fun stateColor(v: Double, panel: DashPanel): Color {
    return when (panel.unit) {
        "percent", "percentunit" -> thresholdColor(v, panel)
        else -> if (v <= 0) Color(0xFFEF4444) else Color(0xFF00A86B)
    }
}

private fun heatColor(t: Float): Color {
    val hue = (1f - t) * 220f
    return Color.hsv(hue, 0.72f, 0.52f + t * 0.08f)
}

/** 模板变量展示名：优先自定义 label，否则用常见英文名的中文映射，避免芯片显示 instance/device 原文。 */
private fun dashVarDisplayLabel(v: DashVar): String {
    val custom = v.label.trim()
    if (custom.isNotBlank()) return custom
    return when (v.name.trim().lowercase()) {
        "instance" -> "实例"
        "device", "device_name" -> "设备"
        "interface", "ifname", "ifDescr", "ifdescr" -> "接口"
        "host", "hostname", "node" -> "主机"
        "job" -> "任务"
        "pod" -> "Pod"
        "service" -> "服务"
        "container" -> "容器"
        "namespace" -> "命名空间"
        "cluster" -> "集群"
        "region", "zone" -> "区域"
        "mountpoint", "mount" -> "挂载点"
        "disk", "device_disk" -> "磁盘"
        "cpu" -> "CPU"
        "gpu" -> "GPU"
        "env", "environment" -> "环境"
        "system" -> "系统"
        "endpoint" -> "接口"
        "app", "application" -> "应用"
        else -> v.name.ifBlank { "变量" }
    }
}

/* ── table 标签→值 ── */
@Composable
private fun TablePanel(values: List<DashValue>, panel: DashPanel) {
    if (values.isEmpty()) { PanelHint("无数据"); return }
    val legends = remember(values) { dashLegends(values.map { it.labels to it.legendFmt }) }
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        values.indices.take(40).forEach { i ->
            Row(Modifier.fillMaxWidth(), Arrangement.SpaceBetween, Alignment.CenterVertically) {
                Text(legends.getOrElse(i) { "-" }.ifBlank { "-" }, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
                Text(fmtUnit(values[i].value, panel.unit, panel.decimals), fontSize = 11.sp, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface)
            }
            HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant.copy(alpha = 0.4f), thickness = 0.5.dp)
        }
    }
}

/* ── 图例 ── */
private val prettyLabelKeys = listOf("name", "endpoint", "system", "instance", "host", "hostname", "device", "mountpoint", "pod", "service", "container", "node")
private val hiddenLabelKeys = setOf("__name__", "id", "job", "check_type", "type")

private fun legendFor(fmt: String, labels: Map<String, String>?): String {
    val lb = labels ?: emptyMap()
    if (fmt.isNotBlank()) {
        val out = Regex("\\{\\{\\s*(\\w+)\\s*\\}\\}").replace(fmt) { m -> lb[m.groupValues[1]] ?: "" }.trim()
        if (out.isNotBlank()) return out
    }
    // 无模板：优先一个可读标签值；隐藏 id/内部类标签，绝不展示 key=value 原样。
    for (k in prettyLabelKeys) lb[k]?.takeIf { it.isNotBlank() }?.let { return it }
    val vals = lb.filterKeys { it !in hiddenLabelKeys && !it.endsWith("_id") }.values.filter { it.isNotBlank() }
    if (vals.isNotEmpty()) return vals.joinToString(" · ")
    return lb["__name__"] ?: ""
}

/** 多序列图例去重可辨：若各图例已互不相同则原样用；否则改用「序列间取值不同的标签」。 */
private fun dashLegends(items: List<Pair<Map<String, String>?, String>>): List<String> {
    val raw = items.map { legendFor(it.second, it.first) }
    if (raw.toSet().size == raw.size) return raw
    val keys = items.flatMap { it.first?.keys.orEmpty() }.filter { it !in hiddenLabelKeys && !it.endsWith("_id") }.toSet()
    val varying = keys.filter { k -> items.map { it.first?.get(k) ?: "" }.toSet().size > 1 }
    return items.mapIndexed { i, c ->
        if (varying.isNotEmpty()) {
            val lbl = varying.mapNotNull { c.first?.get(it) }.filter { it.isNotBlank() }.joinToString(" · ")
            if (lbl.isNotBlank()) return@mapIndexed lbl
        }
        (raw[i].ifBlank { "series" }) + " #${i + 1}"
    }
}

/* ── 单位/数值格式化 ── */
private fun humanShort(v: Double): String {
    if (v.isNaN()) return "-"
    val a = kotlin.math.abs(v)
    return when {
        a >= 1_000_000_000 -> "%.2fB".format(v / 1_000_000_000)
        a >= 1_000_000 -> "%.2fM".format(v / 1_000_000)
        a >= 1_000 -> "%.2fK".format(v / 1_000)
        a >= 100 -> "%.0f".format(v)
        a >= 1 -> "%.2f".format(v)
        a == 0.0 -> "0"
        else -> "%.3f".format(v)
    }
}

private fun humanBytes(v: Double): String {
    val u = arrayOf("B", "KB", "MB", "GB", "TB", "PB"); var x = kotlin.math.abs(v); var i = 0
    while (x >= 1024 && i < u.size - 1) { x /= 1024; i++ }
    val s = if (i == 0) "%.0f".format(x) else "%.2f".format(x)
    return (if (v < 0) "-" else "") + s + " " + u[i]
}

private fun fmtUnit(v: Double, unit: String, decimals: Int): String {
    if (v.isNaN()) return "-"
    fun dec(x: Double, def: Int): String = "%.${if (decimals in 0..6) decimals else def}f".format(x)
    return when (unit) {
        "percent" -> dec(v, 1) + "%"
        "percentunit" -> dec(v * 100, 1) + "%"
        "bytes" -> humanBytes(v)
        "Bps", "bytes/sec" -> humanBytes(v) + "/s"
        "s" -> if (kotlin.math.abs(v) < 1) "%.0f ms".format(v * 1000) else dec(v, 2) + " s"
        "ms" -> dec(v, 0) + " ms"
        "short", "none", "" -> humanShort(v)
        else -> humanShort(v)
    }
}
