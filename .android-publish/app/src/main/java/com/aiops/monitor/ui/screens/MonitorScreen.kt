package com.aiops.monitor.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.History
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.DeleteOutline
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Language
import androidx.compose.material.icons.filled.Link
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.SearchOff
import androidx.compose.material.icons.filled.Security
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material3.AssistChip
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Tab
import androidx.compose.material3.TabRow
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.viewmodel.compose.viewModel
import com.aiops.monitor.data.ApiClient
import com.aiops.monitor.data.SessionTicker
import com.aiops.monitor.data.models.ApiEndpoint
import com.aiops.monitor.data.models.ApiSystem
import com.aiops.monitor.data.models.Probe
import com.aiops.monitor.data.models.CheckUpsertRequest
import com.aiops.monitor.ui.components.LoadingBox
import com.aiops.monitor.ui.components.HistoryChartCard
import com.aiops.monitor.ui.components.StateBox
import com.aiops.monitor.ui.components.StatusDot
import com.aiops.monitor.ui.components.StatusPill
import com.aiops.monitor.ui.components.AppBlue
import com.aiops.monitor.ui.components.AppGreen
import com.aiops.monitor.ui.components.AppOrange
import com.aiops.monitor.ui.components.AppRed
import com.aiops.monitor.ui.viewmodel.MonitorViewModel
import com.aiops.monitor.ui.viewmodel.ChartTimeRange
import com.aiops.monitor.data.models.CheckPoint
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Surface
import androidx.compose.material.icons.filled.Close
import androidx.compose.ui.window.DialogProperties
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

private val MonitorBlue = AppBlue
private val MonitorGreen = AppGreen
private val MonitorRed = AppRed
private val MonitorOrange = AppOrange

private enum class MonitorQuickFilter(val label: String) {
    All("全部对象"), Healthy("正常"), Failing("异常"), Checked("已探测"), Certificates("HTTPS 证书")
}

@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
fun ProbeContent(modifier: Modifier = Modifier, refreshSignal: Int = 0) {
    val vm: MonitorViewModel = viewModel()
    val probes by vm.probes.collectAsState()
    val apiSystems by vm.apiSystems.collectAsState()
    val loading by vm.loading.collectAsState()
    val error by vm.error.collectAsState()
    val testingIds by vm.testingIds.collectAsState()
    val trend by vm.trend.collectAsState()
    val trendLoading by vm.trendLoading.collectAsState()
    val trendTimeRange by vm.trendTimeRange.collectAsState()
    val hostMap by vm.hostMap.collectAsState()
    var selectedTab by remember { mutableIntStateOf(0) }
    var quickFilter by remember { androidx.compose.runtime.mutableStateOf(MonitorQuickFilter.All) }
    var typeFilter by remember { androidx.compose.runtime.mutableStateOf<String?>(null) }
    // 拨测增删改
    var showEditor by remember { androidx.compose.runtime.mutableStateOf(false) }
    var editorInitial by remember { androidx.compose.runtime.mutableStateOf<Probe?>(null) }
    var pendingDeleteProbe by remember { androidx.compose.runtime.mutableStateOf<Probe?>(null) }
    var pendingDeleteSystem by remember { androidx.compose.runtime.mutableStateOf<ApiSystem?>(null) }
    var showDnsThresholds by remember { androidx.compose.runtime.mutableStateOf(false) }
    val dnsWarnMs by vm.dnsWarnMs.collectAsState()
    val dnsCritMs by vm.dnsCritMs.collectAsState()
    val isAdmin by vm.isAdmin.collectAsState()
    val listState = rememberLazyListState()
    val scope = rememberCoroutineScope()

    val visibleProbes = remember(probes, quickFilter, typeFilter) {
        probes.filter { probe ->
            val typeMatch = typeFilter == null || probe.type.equals(typeFilter, ignoreCase = true)
            val quickMatch = when (quickFilter) {
                MonitorQuickFilter.All -> true
                MonitorQuickFilter.Healthy -> probe.checked_at > 0 && probe.ok
                MonitorQuickFilter.Failing -> probe.checked_at > 0 && !probe.ok
                MonitorQuickFilter.Checked -> probe.checked_at > 0
                MonitorQuickFilter.Certificates -> probe.target.isHttpsUrl()
            }
            typeMatch && quickMatch
        }
    }
    val probeTypes = remember(probes) {
        probes.map { it.type.uppercase() }.distinct().sorted()
    }
    val visibleSystems = remember(apiSystems, quickFilter) {
        apiSystems.mapNotNull { system ->
            val endpoints = system.endpoints.filter { endpoint ->
                when (quickFilter) {
                    MonitorQuickFilter.All -> true
                    MonitorQuickFilter.Healthy -> endpoint.checked_at > 0 && endpoint.ok
                    MonitorQuickFilter.Failing -> endpoint.checked_at > 0 && !endpoint.ok
                    MonitorQuickFilter.Checked -> endpoint.checked_at > 0
                    MonitorQuickFilter.Certificates -> endpoint.url.isHttpsUrl()
                }
            }
            system.copy(endpoints = endpoints).takeIf { endpoints.isNotEmpty() || quickFilter == MonitorQuickFilter.All }
        }
    }

    fun openMetric(filter: MonitorQuickFilter) {
        quickFilter = filter
        val hasProbeMatch = probes.any { probe ->
            when (filter) {
                MonitorQuickFilter.All -> true
                MonitorQuickFilter.Healthy -> probe.checked_at > 0 && probe.ok
                MonitorQuickFilter.Failing -> probe.checked_at > 0 && !probe.ok
                MonitorQuickFilter.Checked -> probe.checked_at > 0
                MonitorQuickFilter.Certificates -> probe.target.isHttpsUrl()
            }
        }
        val hasEndpointMatch = apiSystems.any { system ->
            system.endpoints.any { endpoint ->
                when (filter) {
                    MonitorQuickFilter.All -> true
                    MonitorQuickFilter.Healthy -> endpoint.checked_at > 0 && endpoint.ok
                    MonitorQuickFilter.Failing -> endpoint.checked_at > 0 && !endpoint.ok
                    MonitorQuickFilter.Checked -> endpoint.checked_at > 0
                    MonitorQuickFilter.Certificates -> endpoint.url.isHttpsUrl()
                }
            }
        }
        selectedTab = when {
            hasProbeMatch -> 0
            hasEndpointMatch -> 1
            else -> selectedTab
        }
        val firstDetailIndex = if (error != null) 3 else 2
        scope.launch {
            delay(60)
            listState.animateScrollToItem(firstDetailIndex)
        }
    }

    LaunchedEffect(Unit) {
        SessionTicker.pollWhileAlive(15_000L) { vm.load() }
    }
    // InfraHub 顶栏刷新：signal 递增即触发一次即时刷新（初始 0 不重复拉取）。
    LaunchedEffect(refreshSignal) { if (refreshSignal > 0) vm.load() }

    Box(modifier.fillMaxSize()) {
        when {
            error != null && probes.isEmpty() && apiSystems.isEmpty() -> StateBox(
                message = error ?: "加载失败",
                onRetry = vm::load,
                modifier = Modifier.fillMaxSize()
            )
            loading && probes.isEmpty() && apiSystems.isEmpty() -> LoadingBox(
                modifier = Modifier.fillMaxSize()
            )
            else -> LazyColumn(
                state = listState,
                modifier = Modifier.fillMaxSize().padding(horizontal = 14.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp),
                contentPadding = PaddingValues(bottom = 20.dp)
            ) {
                item { MonitorOverview(probes, apiSystems, quickFilter, ::openMetric) }

                if (error != null) {
                    item {
                        Card(
                            colors = CardDefaults.cardColors(containerColor = MonitorOrange.copy(alpha = 0.09f)),
                            shape = RoundedCornerShape(10.dp)
                        ) {
                            Text(
                                error ?: "部分数据加载失败",
                                color = MonitorOrange,
                                style = MaterialTheme.typography.bodySmall,
                                modifier = Modifier.fillMaxWidth().padding(10.dp)
                            )
                        }
                    }
                }

                item {
                    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                        TabRow(
                            selectedTabIndex = selectedTab,
                            containerColor = MaterialTheme.colorScheme.surfaceVariant,
                            contentColor = MonitorBlue,
                            divider = {}
                        ) {
                            Tab(
                                selected = selectedTab == 0,
                                onClick = { selectedTab = 0 },
                                text = { Text("拨测监控  ${visibleProbes.size}", fontWeight = FontWeight.Bold) }
                            )
                            Tab(
                                selected = selectedTab == 1,
                                onClick = { selectedTab = 1 },
                                text = { Text("API 业务监控  ${visibleSystems.size}", fontWeight = FontWeight.Bold) }
                            )
                        }
                        // 拨测类型筛选
                        if (selectedTab == 0 && probeTypes.isNotEmpty()) {
                            Row(
                                modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(vertical = 2.dp),
                                horizontalArrangement = Arrangement.spacedBy(6.dp)
                            ) {
                                FilterChip(
                                    selected = typeFilter == null,
                                    onClick = { typeFilter = null },
                                    label = { Text("全部", fontSize = 11.sp) }
                                )
                                probeTypes.forEach { type ->
                                    FilterChip(
                                        selected = typeFilter == type,
                                        onClick = { typeFilter = if (typeFilter == type) null else type },
                                        label = { Text(type, fontSize = 11.sp) }
                                    )
                                }
                            }
                        }
                        if (quickFilter != MonitorQuickFilter.All) {
                            AssistChip(
                                onClick = { quickFilter = MonitorQuickFilter.All },
                                label = { Text("快捷筛选：${quickFilter.label} · 点击清除", fontSize = 10.sp) }
                            )
                        }
                    }
                }

                if (selectedTab == 0) {
                    item {
                        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            OutlinedButton(onClick = { editorInitial = null; showEditor = true }, modifier = Modifier.weight(1f)) {
                                Icon(Icons.Default.Add, null, modifier = Modifier.size(18.dp))
                                Spacer(Modifier.width(6.dp))
                                Text("新建拨测")
                            }
                            if (isAdmin) {
                                TextButton(onClick = { vm.loadDnsThresholds(); showDnsThresholds = true }) {
                                    Text("DNS 阈值", fontSize = 13.sp)
                                }
                            }
                        }
                    }
                    if (visibleProbes.isEmpty()) {
                        item { EmptyMonitorCard("没有${quickFilter.label}拨测", "点击上方筛选标签可恢复全部监控对象") }
                    } else {
                        items(visibleProbes, key = { it.id }) { probe ->
                            ProbeCard(
                                probe = probe,
                                hostMap = hostMap,
                                isTesting = "probe:${probe.id}" in testingIds,
                                onTest = { vm.testProbe(probe.id) },
                                onHistory = { vm.loadProbeHistory(probe.id, probe.name) },
                                onEdit = { editorInitial = probe; showEditor = true },
                                onDelete = { pendingDeleteProbe = probe }
                            )
                        }
                    }
                } else {
                    if (visibleSystems.isEmpty()) {
                        item { EmptyMonitorCard("没有${quickFilter.label} API", "点击上方筛选标签可恢复全部业务系统") }
                    } else {
                        items(visibleSystems, key = { it.id }) { system ->
                            ApiSystemCard(
                                system = system,
                                isTesting = "system:${system.id}" in testingIds,
                                onTest = { vm.testApiSystem(system.id) },
                                onHistory = vm::loadApiHistory,
                                onDelete = { pendingDeleteSystem = system }
                            )
                        }
                    }
                }
            }
        }

        if (trendLoading) {
            AlertDialog(
                onDismissRequest = {},
                containerColor = MaterialTheme.colorScheme.surface,
                shape = RoundedCornerShape(16.dp),
                confirmButton = {},
                title = { Text("正在加载历史趋势", fontSize = 16.sp, fontWeight = FontWeight.Bold) },
                text = {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        CircularProgressIndicator(Modifier.size(22.dp), strokeWidth = 2.dp)
                        Spacer(Modifier.width(12.dp))
                        Text("正在读取采样数据…", fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                }
            )
        }
        trend?.let { selected ->
            if (selected.points.size >= 2) {
                CheckHistoryDialog(
                    title = selected.title,
                    points = selected.points,
                    timeRange = trendTimeRange,
                    onTimeRangeChange = vm::setTrendTimeRange,
                    onDismiss = vm::closeTrend
                )
            } else {
                AlertDialog(
                    onDismissRequest = vm::closeTrend,
                    containerColor = MaterialTheme.colorScheme.surface,
                    shape = RoundedCornerShape(16.dp),
                    title = { Text(selected.title, fontSize = 16.sp, fontWeight = FontWeight.Bold) },
                    text = { Text("历史采样点不足，至少积累 2 个采样点后才能绘制趋势。", fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurfaceVariant) },
                    confirmButton = { TextButton(onClick = vm::closeTrend) { Text("知道了", fontSize = 13.sp) } }
                )
            }
        }

        if (showEditor) {
            CheckEditDialog(
                initial = editorInitial,
                onSave = { req -> vm.upsertProbe(req); showEditor = false },
                onDismiss = { showEditor = false }
            )
        }
        if (showDnsThresholds) {
            DnsThresholdDialog(
                warnMs = dnsWarnMs, critMs = dnsCritMs,
                onSave = { w, c -> vm.saveDnsThresholds(w, c) { showDnsThresholds = false } },
                onDismiss = { showDnsThresholds = false }
            )
        }
        pendingDeleteProbe?.let { p ->
            AlertDialog(
                onDismissRequest = { pendingDeleteProbe = null },
                title = { Text("删除拨测") },
                text = { Text("确定删除拨测「${p.name}」吗？此操作不可撤销。") },
                confirmButton = { Button(onClick = { vm.deleteProbe(p.id); pendingDeleteProbe = null }, colors = ButtonDefaults.buttonColors(containerColor = MonitorRed)) { Text("删除") } },
                dismissButton = { TextButton(onClick = { pendingDeleteProbe = null }) { Text("取消") } }
            )
        }
        pendingDeleteSystem?.let { sys ->
            AlertDialog(
                onDismissRequest = { pendingDeleteSystem = null },
                title = { Text("删除业务系统") },
                text = { Text("确定删除「${sys.name}」及其所有接口吗？此操作不可撤销。") },
                confirmButton = { Button(onClick = { vm.deleteApiSystem(sys.id); pendingDeleteSystem = null }, colors = ButtonDefaults.buttonColors(containerColor = MonitorRed)) { Text("删除") } },
                dismissButton = { TextButton(onClick = { pendingDeleteSystem = null }) { Text("取消") } }
            )
        }
    }
}

@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
private fun CheckEditDialog(initial: Probe?, onSave: (CheckUpsertRequest) -> Unit, onDismiss: () -> Unit) {
    var name by androidx.compose.runtime.remember { androidx.compose.runtime.mutableStateOf(initial?.name ?: "") }
    var type by androidx.compose.runtime.remember { androidx.compose.runtime.mutableStateOf(initial?.type ?: "http") }
    var target by androidx.compose.runtime.remember { androidx.compose.runtime.mutableStateOf(initial?.target ?: "") }
    var interval by androidx.compose.runtime.remember { androidx.compose.runtime.mutableStateOf((initial?.interval_sec ?: 30).toString()) }
    var level by androidx.compose.runtime.remember { androidx.compose.runtime.mutableStateOf(initial?.level ?: "critical") }
    var enabled by androidx.compose.runtime.remember { androidx.compose.runtime.mutableStateOf(initial?.enabled ?: true) }
    var dnsType by androidx.compose.runtime.remember { androidx.compose.runtime.mutableStateOf(initial?.dns_type?.takeIf { it.isNotBlank() } ?: "A") }
    var expectKeyword by androidx.compose.runtime.remember { androidx.compose.runtime.mutableStateOf(initial?.expect_keyword ?: "") }
    var timeout by androidx.compose.runtime.remember { androidx.compose.runtime.mutableStateOf((initial?.timeout_sec ?: 0).let { if (it > 0) it.toString() else "" }) }
    val isDns = type == "dns"
    val targetHint = when (type) {
        "http" -> "URL，如 https://example.com"
        "tcp", "udp" -> "host:port，如 10.0.0.1:6379"
        "ping" -> "主机名或 IP"
        "dns" -> "域名，如 example.com（可加 @8.8.8.8 指定 DNS）"
        "process" -> "hostID/进程名"
        else -> "目标"
    }
    AlertDialog(
        onDismissRequest = onDismiss,
        containerColor = MaterialTheme.colorScheme.surface,
        title = { Text(if (initial == null) "新建拨测" else "编辑拨测", fontWeight = FontWeight.Bold) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                OutlinedTextField(value = name, onValueChange = { name = it }, label = { Text("名称") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                Text("类型", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    listOf("http", "tcp", "ping", "udp", "dns", "process").forEach { t ->
                        FilterChip(selected = type == t, onClick = { type = t }, label = { Text(t.uppercase(), fontSize = 11.sp) })
                    }
                }
                OutlinedTextField(value = target, onValueChange = { target = it }, label = { Text(targetHint) }, singleLine = true, modifier = Modifier.fillMaxWidth())
                if (isDns) {
                    Text("DNS 记录类型", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                        listOf("A", "AAAA", "CNAME", "MX", "TXT", "NS").forEach { dt ->
                            FilterChip(selected = dnsType == dt, onClick = { dnsType = dt }, label = { Text(dt, fontSize = 11.sp) })
                        }
                    }
                    OutlinedTextField(value = expectKeyword, onValueChange = { expectKeyword = it }, label = { Text("期望包含(可选)，如某 IP/域名") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                }
                Row(horizontalArrangement = Arrangement.spacedBy(10.dp), verticalAlignment = Alignment.CenterVertically) {
                    OutlinedTextField(value = interval, onValueChange = { v -> interval = v.filter { it.isDigit() }.take(5) }, label = { Text("间隔(秒)") }, singleLine = true, modifier = Modifier.weight(1f))
                    if (isDns) OutlinedTextField(value = timeout, onValueChange = { v -> timeout = v.filter { it.isDigit() }.take(4) }, label = { Text("超时(秒)") }, singleLine = true, modifier = Modifier.weight(1f))
                }
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text("级别", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    FilterChip(selected = level == "warning", onClick = { level = "warning" }, label = { Text("警告", fontSize = 11.sp) })
                    FilterChip(selected = level == "critical", onClick = { level = "critical" }, label = { Text("严重", fontSize = 11.sp) })
                }
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("启用", Modifier.weight(1f), color = MaterialTheme.colorScheme.onSurface)
                    Switch(checked = enabled, onCheckedChange = { enabled = it })
                }
            }
        },
        confirmButton = {
            Button(
                onClick = {
                    onSave(CheckUpsertRequest(
                        id = initial?.id ?: "", name = name.trim(), type = type, target = target.trim(),
                        interval_sec = interval.toIntOrNull() ?: 30, level = level, enabled = enabled,
                        dns_type = if (isDns) dnsType else "A",
                        expect_keyword = if (isDns) expectKeyword.trim() else "",
                        timeout_sec = if (isDns) (timeout.toIntOrNull() ?: 0) else 0
                    ))
                },
                enabled = name.isNotBlank() && target.isNotBlank()
            ) { Text("保存") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("取消") } }
    )
}

// 拨测/API 业务监控 历史多图弹窗：分位汇总 + 响应延迟 + 可用性 + 延迟分布直方图（对齐 web 端 4 图）。
@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
private fun CheckHistoryDialog(
    title: String,
    points: List<CheckPoint>,
    timeRange: ChartTimeRange,
    onTimeRangeChange: (ChartTimeRange) -> Unit,
    onDismiss: () -> Unit
) {
    val sorted = androidx.compose.runtime.remember(points) { points.sortedBy { it.timestamp } }
    val timestamps = sorted.map { it.timestamp }
    val latencies = sorted.map { it.latency_ms }
    val validLat = androidx.compose.runtime.remember(sorted) { latencies.filter { it >= 0 }.sorted() }
    val avg = if (validLat.isNotEmpty()) validLat.average() else 0.0
    fun percentile(p: Double): Double = if (validLat.isEmpty()) 0.0 else validLat[((validLat.size - 1) * p).toInt().coerceIn(0, validLat.lastIndex)]
    val p95 = percentile(0.95); val p99 = percentile(0.99)
    val uptime = if (sorted.isNotEmpty()) sorted.count { it.ok } * 100.0 / sorted.size else 0.0
    val pointDetails = sorted.map { p -> (if (p.ok) "成功" else "失败") + (if (p.status_code > 0) " · HTTP ${p.status_code}" else "") }
    var fullscreen by remember { mutableStateOf<ChartFullscreenSpec?>(null) }

    AlertDialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false),
        modifier = Modifier.fillMaxSize(),
        confirmButton = {},
        title = null,
        text = {
            Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                Row(Modifier.fillMaxWidth(), Arrangement.SpaceBetween, Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        Text(title, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Text("延迟 · 可用性 · 分布 · 分位", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    IconButton(onClick = onDismiss) { Icon(Icons.Default.Close, "关闭", tint = MaterialTheme.colorScheme.onSurfaceVariant) }
                }
                TimeRangeSelector(timeRange, onTimeRangeChange)
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    HistStat("平均", "%.0f ms".format(avg), MonitorBlue, Modifier.weight(1f))
                    HistStat("P95", "%.0f ms".format(p95), MonitorOrange, Modifier.weight(1f))
                    HistStat("P99", "%.0f ms".format(p99), MonitorRed, Modifier.weight(1f))
                    HistStat("可用率", "%.1f%%".format(uptime), if (uptime >= 99) MonitorGreen else MonitorRed, Modifier.weight(1f))
                }
                HistoryChartCard(
                    title = "① 响应延迟",
                    values = latencies,
                    timestamps = timestamps,
                    unit = " ms",
                    color = MonitorBlue,
                    pointDetails = pointDetails,
                    onExpand = {
                        fullscreen = ChartFullscreenSpec("响应延迟", latencies, timestamps, " ms", MonitorBlue, pointDetails)
                    }
                )
                HistoryChartCard(
                    title = "② 可用性（成功=100 / 失败=0）",
                    values = sorted.map { if (it.ok) 100.0 else 0.0 },
                    timestamps = timestamps,
                    unit = " %",
                    color = MonitorGreen,
                    onExpand = {
                        fullscreen = ChartFullscreenSpec(
                            "可用性",
                            sorted.map { if (it.ok) 100.0 else 0.0 },
                            timestamps,
                            " %",
                            MonitorGreen
                        )
                    }
                )
                Text("③ 延迟分布直方图（看长尾 / 双峰）", fontWeight = FontWeight.Bold, fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurface)
                LatencyHistogram(validLat)
                if (sorted.any { it.resp_bytes > 0 }) {
                    val sizes = sorted.map { it.resp_bytes / 1024.0 }
                    HistoryChartCard(
                        title = "④ 响应体大小（KB）",
                        values = sizes,
                        timestamps = timestamps,
                        unit = " KB",
                        color = Color(0xFF8B5CF6),
                        onExpand = {
                            fullscreen = ChartFullscreenSpec("响应体大小", sizes, timestamps, " KB", Color(0xFF8B5CF6))
                        }
                    )
                }
                Spacer(Modifier.height(24.dp))
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
            pointDetails = fs.pointDetails,
            onDismiss = { fullscreen = null }
        )
    }
}

private data class ChartFullscreenSpec(
    val title: String,
    val values: List<Double>,
    val timestamps: List<Long>,
    val unit: String,
    val color: Color,
    val pointDetails: List<String> = emptyList(),
)

@Composable
private fun HistStat(label: String, value: String, color: Color, modifier: Modifier = Modifier) {
    Surface(modifier, color = MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.4f), shape = RoundedCornerShape(10.dp)) {
        Column(Modifier.padding(vertical = 8.dp), horizontalAlignment = Alignment.CenterHorizontally) {
            Text(value, fontWeight = FontWeight.Bold, color = color, fontSize = 13.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Text(label, fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

@Composable
private fun LatencyHistogram(sortedLatencies: List<Double>) {
    if (sortedLatencies.isEmpty()) { Text("暂无延迟数据", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant); return }
    val bounds = listOf(50.0, 100.0, 200.0, 500.0, 1000.0, 2000.0, 5000.0)
    val labels = listOf("<50", "50-100", "100-200", "200-500", "500ms-1s", "1-2s", "2-5s", ">5s")
    val counts = IntArray(labels.size)
    sortedLatencies.forEach { v ->
        var idx = bounds.indexOfFirst { v < it }
        if (idx < 0) idx = labels.lastIndex
        counts[idx]++
    }
    val maxCount = (counts.maxOrNull() ?: 1).coerceAtLeast(1)
    Column(verticalArrangement = Arrangement.spacedBy(5.dp)) {
        labels.forEachIndexed { i, label ->
            val c = counts[i]
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(label, fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.width(66.dp))
                Surface(Modifier.weight(1f).height(16.dp), color = MaterialTheme.colorScheme.surfaceVariant, shape = RoundedCornerShape(4.dp)) {
                    if (c > 0) Box(Modifier.fillMaxWidth((c.toFloat() / maxCount).coerceIn(0.02f, 1f)).fillMaxHeight().background(MonitorBlue.copy(alpha = 0.75f)))
                }
                Text("$c", fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurface, modifier = Modifier.width(30.dp), textAlign = androidx.compose.ui.text.style.TextAlign.End)
            }
        }
    }
}

@Composable
private fun DnsThresholdDialog(warnMs: Double, critMs: Double, onSave: (Double, Double) -> Unit, onDismiss: () -> Unit) {
    var warn by androidx.compose.runtime.remember(warnMs) { androidx.compose.runtime.mutableStateOf((if (warnMs > 0) warnMs else 500.0).toInt().toString()) }
    var crit by androidx.compose.runtime.remember(critMs) { androidx.compose.runtime.mutableStateOf((if (critMs > 0) critMs else 2000.0).toInt().toString()) }
    AlertDialog(
        onDismissRequest = onDismiss,
        containerColor = MaterialTheme.colorScheme.surface,
        title = { Text("DNS 解析延迟阈值", fontWeight = FontWeight.Bold) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Text("DNS 拨测的解析延迟达到阈值即产生对应级别的延迟告警（全局，作用于所有 DNS 拨测）。", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                OutlinedTextField(value = warn, onValueChange = { v -> warn = v.filter { it.isDigit() }.take(6) }, label = { Text("警告阈值 (ms)") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                OutlinedTextField(value = crit, onValueChange = { v -> crit = v.filter { it.isDigit() }.take(6) }, label = { Text("严重阈值 (ms)") }, singleLine = true, modifier = Modifier.fillMaxWidth())
            }
        },
        confirmButton = {
            Button(
                onClick = { onSave(warn.toDoubleOrNull() ?: 500.0, crit.toDoubleOrNull() ?: 2000.0) },
                enabled = warn.isNotBlank() && crit.isNotBlank()
            ) { Text("保存") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("取消") } }
    )
}

@Composable
private fun MonitorOverview(
    probes: List<Probe>,
    systems: List<ApiSystem>,
    selected: MonitorQuickFilter,
    onMetricClick: (MonitorQuickFilter) -> Unit
) {
    val endpoints = systems.flatMap { it.endpoints }
    val checkedProbes = probes.count { it.checked_at > 0 }
    val healthy = probes.count { it.checked_at > 0 && it.ok } + endpoints.count { it.checked_at > 0 && it.ok }
    val failing = probes.count { it.checked_at > 0 && !it.ok } + endpoints.count { it.checked_at > 0 && !it.ok }
    val httpsCertificates = probes.filter { it.target.isHttpsUrl() && it.cert_days >= 0 }.map { it.cert_days } +
        endpoints.filter { it.url.isHttpsUrl() && it.cert_days >= 0 }.map { it.cert_days }
    // 原来 6 张大指标卡（占 3 行）→ 一行可横滑的筛选 chip，计数与筛选都保留，版面省一大截。
    // 证书「最近到期」是唯一非筛选信息，单独放一行小字（带到期紧迫度着色）。
    val nearest = httpsCertificates.minOrNull()
    Column(modifier = Modifier.fillMaxWidth().padding(top = 6.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Row(
            Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            MonSummaryChip("全部对象", probes.size + endpoints.size, selected == MonitorQuickFilter.All) { onMetricClick(MonitorQuickFilter.All) }
            MonSummaryChip("正常", healthy, selected == MonitorQuickFilter.Healthy) { onMetricClick(MonitorQuickFilter.Healthy) }
            MonSummaryChip("异常", failing, selected == MonitorQuickFilter.Failing, danger = failing > 0) { onMetricClick(MonitorQuickFilter.Failing) }
            MonSummaryChip("已探测", checkedProbes + endpoints.count { it.checked_at > 0 }, selected == MonitorQuickFilter.Checked) { onMetricClick(MonitorQuickFilter.Checked) }
            MonSummaryChip("HTTPS 证书", httpsCertificates.size, selected == MonitorQuickFilter.Certificates) { onMetricClick(MonitorQuickFilter.Certificates) }
        }
        if (nearest != null) {
            Text(
                "HTTPS 证书最近到期 $nearest 天",
                color = certificateColor(nearest),
                style = MaterialTheme.typography.labelMedium,
                fontWeight = FontWeight.Medium
            )
        }
    }
}

@Composable
private fun MonSummaryChip(label: String, count: Int, selected: Boolean, danger: Boolean = false, onClick: () -> Unit) {
    FilterChip(
        selected = selected,
        onClick = onClick,
        label = { Text("$label $count", fontWeight = FontWeight.Medium) },
        colors = FilterChipDefaults.filterChipColors(
            selectedContainerColor = (if (danger) MonitorRed else MonitorBlue).copy(alpha = 0.16f),
            selectedLabelColor = if (danger) MonitorRed else MonitorBlue
        )
    )
}

@Composable
private fun SummaryTile(
    label: String,
    value: String,
    color: Color,
    selected: Boolean,
    modifier: Modifier,
    onClick: () -> Unit
) {
    Card(
        onClick = onClick,
        modifier = modifier.border(
            1.dp,
            if (selected) color.copy(alpha = 0.4f) else MaterialTheme.colorScheme.outlineVariant,
            RoundedCornerShape(14.dp)
        ),
        shape = RoundedCornerShape(14.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 0.dp)
    ) {
        Column(Modifier.fillMaxWidth().padding(vertical = 12.dp), horizontalAlignment = Alignment.CenterHorizontally) {
            Text(value, color = color, fontWeight = FontWeight.Black, fontSize = 22.sp)
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(label, color = MaterialTheme.colorScheme.onSurfaceVariant, style = MaterialTheme.typography.labelSmall)
                Spacer(Modifier.width(3.dp))
                Icon(Icons.AutoMirrored.Filled.ArrowForward, "查看$label", tint = color, modifier = Modifier.size(11.dp))
            }
        }
    }
}

@Composable
private fun ProbeCard(probe: Probe, hostMap: Map<String, String>, isTesting: Boolean, onTest: () -> Unit, onHistory: () -> Unit, onEdit: () -> Unit = {}, onDelete: () -> Unit = {}) {
    val stateColor = when {
        !probe.enabled || probe.checked_at <= 0 -> MaterialTheme.colorScheme.onSurfaceVariant
        probe.ok -> MonitorGreen
        else -> MonitorRed
    }
    val stateText = when {
        !probe.enabled -> "已停用"
        probe.checked_at <= 0 -> "未探测"
        probe.ok -> "正常"
        else -> "异常"
    }
    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(14.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 0.dp)
    ) {
        Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min)) {
            Box(Modifier.width(4.dp).fillMaxHeight().background(stateColor))
            Column(Modifier.weight(1f).padding(14.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                // 头部行：名称 + 类型标签 + 操作按钮
                Row(verticalAlignment = Alignment.CenterVertically) {
                    StatusDot(stateColor, 9.dp)
                    Spacer(Modifier.width(8.dp))
                    Column(Modifier.weight(1f)) {
                        Text(probe.name, color = MaterialTheme.colorScheme.onSurface, fontWeight = FontWeight.Bold, maxLines = 1)
                        if (probe.builtin) Text("内置自检", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
                    }
                    StatusPill(probe.type.uppercase(), typeColor(probe.type))
                    Spacer(Modifier.width(8.dp))
                    // 操作按钮移到头部行
                    IconButton(onClick = onHistory, modifier = Modifier.size(32.dp)) {
                        Icon(Icons.Default.History, "24h 趋势", tint = MonitorBlue, modifier = Modifier.size(16.dp))
                    }
                    IconButton(onClick = onTest, enabled = !isTesting && probe.enabled, modifier = Modifier.size(32.dp)) {
                        if (isTesting) CircularProgressIndicator(Modifier.size(14.dp), strokeWidth = 2.dp, color = MonitorBlue)
                        else Icon(Icons.Default.PlayArrow, "立即探测", tint = MonitorBlue, modifier = Modifier.size(18.dp))
                    }
                    if (!probe.builtin) {
                        IconButton(onClick = onEdit, modifier = Modifier.size(32.dp)) {
                            Icon(Icons.Default.Edit, "编辑", tint = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.size(16.dp))
                        }
                        IconButton(onClick = onDelete, modifier = Modifier.size(32.dp)) {
                            Icon(Icons.Default.DeleteOutline, "删除", tint = MonitorRed, modifier = Modifier.size(16.dp))
                        }
                    }
                }
                // 目标地址行：进程类型显示"主机名称/进程名"（从 hostMap 查找）
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(Icons.Default.Link, null, tint = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.size(14.dp))
                    Spacer(Modifier.width(5.dp))
                    val displayTarget = if (probe.type.lowercase() == "process") {
                        val slashIdx = probe.target.indexOf('/')
                        if (slashIdx > 0) {
                            val hostId = probe.target.substring(0, slashIdx)
                            val processName = probe.target.substring(slashIdx + 1)
                            val hostname = hostMap[hostId]
                            if (hostname != null) "$hostname/$processName" else probe.target
                        } else {
                            probe.target
                        }
                    } else {
                        probe.target
                    }
                    Text(displayTarget, color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                }
                // 指标行：延迟 + 类型特有指标 + 上次探测 + 探测结果
                Row(horizontalArrangement = Arrangement.spacedBy(18.dp), verticalAlignment = Alignment.CenterVertically) {
                    MonitorMetric("本次延迟", if (probe.checked_at > 0) "%.1f ms".format(probe.latency_ms) else "—", latencyColor(probe.latency_ms))
                    // HTTP/HTTPS 类型统一展示状态码（含内置监控）
                    if (probe.type.lowercase() in listOf("http", "https") && probe.checked_at > 0) {
                        val code = if (probe.status_code > 0) probe.status_code else 200
                        val codeColor = if (code < 400) MonitorGreen else MonitorRed
                        MonitorMetric("状态码", "$code", codeColor)
                    }
                    // Ping 类型：丢包率
                    if (probe.type.lowercase() == "ping" && probe.checked_at > 0 && probe.loss_pct >= 0) {
                        val lossColor = when {
                            probe.loss_pct == 0.0 -> MonitorGreen
                            probe.loss_pct < 10.0 -> MonitorOrange
                            else -> MonitorRed
                        }
                        MonitorMetric("丢包率", "%.1f%%".format(probe.loss_pct), lossColor)
                    }
                    // TCP 类型：连接状态
                    if (probe.type.lowercase() == "tcp" && probe.checked_at > 0) {
                        val connStatus = if (probe.ok) "已连接" else "未连接"
                        val connColor = if (probe.ok) MonitorGreen else MonitorRed
                        MonitorMetric("连接状态", connStatus, connColor)
                    }
                    // 进程类型：进程名
                    if (probe.type.lowercase() == "process" && probe.checked_at > 0) {
                        val procName = probe.target.substringAfterLast("/").takeIf { it.isNotBlank() && it != probe.target } ?: "—"
                        MonitorMetric("进程名", procName, MonitorBlue)
                    }
                    // DNS 类型：记录类型
                    if (probe.type.lowercase() == "dns") {
                        MonitorMetric("记录", probe.dns_type?.takeIf { it.isNotBlank() }?.uppercase() ?: "A", MonitorBlue)
                    }
                    MonitorMetric("上次探测", formatMonitorTime(probe.checked_at), MaterialTheme.colorScheme.onSurface)
                    // 探测结果
                    MonitorMetric("探测结果", stateText, stateColor)
                    Spacer(Modifier.weight(1f))
                }
                // HTTPS 证书状态
                if (probe.target.isHttpsUrl()) {
                    CertificateStatusRow(probe.cert_days, probe.checked_at)
                }
            }
        }
    }
}

@Composable
private fun ApiSystemCard(
    system: ApiSystem,
    isTesting: Boolean,
    onTest: () -> Unit,
    onHistory: (String, String) -> Unit,
    onDelete: () -> Unit = {}
) {
    val downCount = system.endpoints.count { it.checked_at > 0 && !it.ok }
    val systemColor = when {
        !system.enabled -> MaterialTheme.colorScheme.onSurfaceVariant
        downCount > 0 -> MonitorRed
        else -> MonitorGreen
    }
    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(14.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 0.dp)
    ) {
        Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min)) {
            Box(Modifier.width(4.dp).fillMaxHeight().background(systemColor))
            Column(Modifier.weight(1f).padding(14.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text(system.name, color = MaterialTheme.colorScheme.onSurface, fontWeight = FontWeight.Black, fontSize = 16.sp)
                    Text("${system.endpoints.size} 个接口 · 每 ${system.interval_sec}s", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
                }
                if (!system.enabled) StatusPill("已停用", MaterialTheme.colorScheme.onSurfaceVariant)
                else if (downCount > 0) StatusPill("$downCount 异常", MonitorRed)
                else StatusPill("运行中", MonitorGreen)
                IconButton(onClick = onTest, enabled = system.enabled && !isTesting) {
                    if (isTesting) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp, color = MonitorBlue)
                    else Icon(Icons.Default.PlayArrow, "立即探测", tint = MonitorBlue)
                }
                IconButton(onClick = onDelete, modifier = Modifier.size(36.dp)) {
                    Icon(Icons.Default.DeleteOutline, "删除", tint = MonitorRed, modifier = Modifier.size(18.dp))
                }
            }
            if (system.endpoints.isEmpty()) {
                Text("该业务系统暂无接口", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 12.sp)
            } else {
                system.endpoints.forEachIndexed { index, endpoint ->
                    if (index > 0) HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant, thickness = 0.5.dp)
                    ApiEndpointRow(endpoint) { onHistory(endpoint.id, endpoint.name) }
                }
            }
            }
        }
    }
}

@Composable
private fun ApiEndpointRow(endpoint: ApiEndpoint, onHistory: () -> Unit) {
    val color = when {
        !endpoint.enabled || endpoint.checked_at <= 0 -> MaterialTheme.colorScheme.onSurfaceVariant
        endpoint.ok -> MonitorGreen
        else -> MonitorRed
    }
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            StatusDot(color, 7.dp)
            Spacer(Modifier.width(7.dp))
            Column(Modifier.weight(1f)) {
                Text(endpoint.name, color = MaterialTheme.colorScheme.onSurface, fontWeight = FontWeight.Bold, fontSize = 13.sp)
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(Icons.Default.Language, null, tint = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.size(12.dp))
                    Spacer(Modifier.width(4.dp))
                    Text("${endpoint.method} ${endpoint.url}", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                }
            }
            IconButton(onClick = onHistory, modifier = Modifier.size(34.dp)) {
                Icon(Icons.Default.History, "历史趋势", tint = MonitorBlue, modifier = Modifier.size(18.dp))
            }
        }
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            EndpointMetric("本次", formatMs(endpoint.latency_ms), latencyColor(endpoint.latency_ms), Modifier.weight(1f))
            EndpointMetric("平均 1h", formatMs(endpoint.avg_ms), MonitorBlue, Modifier.weight(1f))
            EndpointMetric("P95 1h", formatMs(endpoint.p95_ms), MonitorOrange, Modifier.weight(1f))
            EndpointMetric("可用率 1h", formatPercent(endpoint.avail_1h), availabilityColor(endpoint.avail_1h), Modifier.weight(1f))
        }
        if (endpoint.url.isHttpsUrl()) {
            CertificateStatusRow(endpoint.cert_days, endpoint.checked_at)
        }
        endpoint.message?.takeIf { it.isNotBlank() }?.let {
            Text(it, color = if (endpoint.ok) MaterialTheme.colorScheme.onSurfaceVariant else MonitorRed, fontSize = 10.sp, maxLines = 2)
        }
    }
}

@Composable
private fun EndpointMetric(label: String, value: String, color: Color, modifier: Modifier) {
    Column(modifier.background(MaterialTheme.colorScheme.surfaceVariant, RoundedCornerShape(10.dp)).padding(vertical = 8.dp), horizontalAlignment = Alignment.CenterHorizontally) {
        Text(value, color = color, fontWeight = FontWeight.Bold, fontSize = 11.sp, maxLines = 1)
        Text(label, color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp, maxLines = 1)
    }
}

@Composable
private fun MonitorMetric(label: String, value: String, color: Color) {
    Column {
        Text(label, color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
        Text(value, color = color, fontSize = 11.sp, fontWeight = FontWeight.Bold, maxLines = 1)
    }
}

@Composable
private fun CertificateStatusRow(days: Int, checkedAt: Long) {
    val color = if (days >= 0) certificateColor(days) else MaterialTheme.colorScheme.onSurfaceVariant
    val value = when {
        checkedAt <= 0 -> "等待首次 HTTPS 握手"
        days < 0 -> "证书有效期暂不可用"
        days == 0 -> "证书今天到期"
        else -> "证书剩余 $days 天"
    }
    Row(
        Modifier.fillMaxWidth().background(color.copy(alpha = 0.09f), RoundedCornerShape(8.dp)).padding(horizontal = 9.dp, vertical = 7.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Icon(Icons.Default.Security, contentDescription = null, tint = color, modifier = Modifier.size(15.dp))
        Spacer(Modifier.width(6.dp))
        Text("HTTPS", color = color, fontSize = 10.sp, fontWeight = FontWeight.Bold)
        Spacer(Modifier.weight(1f))
        Text(value, color = color, fontSize = 11.sp, fontWeight = FontWeight.SemiBold)
    }
}

private fun String.isHttpsUrl(): Boolean = trim().startsWith("https://", ignoreCase = true)

private fun certificateColor(days: Int): Color = when {
    days <= 7 -> MonitorRed
    days <= 30 -> MonitorOrange
    else -> MonitorGreen
}

@Composable
private fun EmptyMonitorCard(title: String, subtitle: String) {
    Card(
        shape = RoundedCornerShape(14.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 0.dp)
    ) {
        Column(
            Modifier.fillMaxWidth().padding(32.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            Icon(Icons.Default.SearchOff, null, tint = MaterialTheme.colorScheme.outline, modifier = Modifier.size(40.dp))
            Text(title, color = MaterialTheme.colorScheme.onSurfaceVariant, fontWeight = FontWeight.Bold)
            Text(subtitle, color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 11.sp)
        }
    }
}

private fun typeColor(type: String): Color = when (type.lowercase()) {
    "http" -> MonitorBlue
    "tcp" -> MonitorOrange
    "ping" -> Color(0xFF8B5CF6)
    "process" -> Color(0xFF06B6D4)
    "dns" -> Color(0xFF10B981)
    else -> Color.Gray
}


@Composable
private fun latencyColor(value: Double): Color = when {
    value <= 0 -> MaterialTheme.colorScheme.onSurfaceVariant
    value < 200 -> MonitorGreen
    value < 1000 -> MonitorOrange
    else -> MonitorRed
}

@Composable
private fun availabilityColor(value: Double): Color = when {
    value < 0 -> MaterialTheme.colorScheme.onSurfaceVariant
    value >= 99.9 -> MonitorGreen
    value >= 99.0 -> MonitorOrange
    else -> MonitorRed
}

private fun formatMs(value: Double) = if (value > 0) "%.0fms".format(value) else "—"
private fun formatPercent(value: Double) = if (value >= 0) "%.2f%%".format(value) else "—"

private fun formatMonitorTime(timestamp: Long): String {
    if (timestamp <= 0) return "—"
    return try {
        val millis = if (timestamp > 1_000_000_000_000L) timestamp else timestamp * 1000
        SimpleDateFormat("HH:mm:ss", Locale.getDefault()).format(Date(millis))
    } catch (_: Exception) {
        "—"
    }
}
