package com.aiops.monitor.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Fullscreen
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Category
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.DialogProperties
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavHostController
import com.aiops.monitor.data.ApiClient
import com.aiops.monitor.data.SessionTicker
import com.aiops.monitor.data.models.DiskInfo
import com.aiops.monitor.ui.components.*
import com.aiops.monitor.ui.Routes
import com.aiops.monitor.ui.formatBytes
import com.aiops.monitor.ui.formatRate
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.rememberScrollState
import com.aiops.monitor.data.models.Sample
import com.aiops.monitor.ui.viewmodel.ChartTimeRange
import com.aiops.monitor.ui.viewmodel.HostDetailViewModel
import androidx.compose.foundation.shape.RoundedCornerShape
import kotlinx.coroutines.delay

private enum class HostChartMetric(val title: String, val color: Color) {
    CPU("CPU 占用率", Color(0xFF4F7FFF)),
    MEM("内存使用率", Color(0xFF00A86B)),
    DISK("磁盘使用率", Color(0xFFF59E0B)),
    GPU_UTIL("GPU 计算利用率", Color(0xFF8B5CF6)),
    GPU_MEM("GPU 显存利用率", Color(0xFF06B6D4));

    fun extractValues(samples: List<Sample>): Pair<List<Double>, List<Long>> {
        return when (this) {
            CPU -> samples.map { it.cpu_percent } to samples.map { it.timestamp }
            MEM -> samples.map { it.mem_percent } to samples.map { it.timestamp }
            DISK -> samples.map { it.disk_percent } to samples.map { it.timestamp }
            GPU_UTIL -> {
                val gpuSamples = samples.filter { !it.gpus.isNullOrEmpty() }
                gpuSamples.map { s -> s.gpus.orEmpty().maxOfOrNull { it.util_percent } ?: 0.0 } to gpuSamples.map { it.timestamp }
            }
            GPU_MEM -> {
                val gpuSamples = samples.filter { !it.gpus.isNullOrEmpty() }
                gpuSamples.map { s -> s.gpus.orEmpty().maxOfOrNull { it.mem_percent } ?: 0.0 } to gpuSamples.map { it.timestamp }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HostDetailScreen(hostId: String, navController: NavHostController, modifier: Modifier = Modifier) {
    val vm: HostDetailViewModel = viewModel()
    val host by vm.host.collectAsState()
    val metrics by vm.metrics.collectAsState()
    val loading by vm.loading.collectAsState()
    val error by vm.error.collectAsState()
    val history by vm.history.collectAsState()
    val historyLoading by vm.historyLoading.collectAsState()

    var showChartMetric by remember { mutableStateOf<HostChartMetric?>(null) }
    var showCategoryDialog by remember { mutableStateOf(false) }
    var showAiAssist by remember { mutableStateOf(false) }
    val vmChartRange by vm.chartTimeRange.collectAsState()

    LaunchedEffect(hostId) {
        SessionTicker.pollWhileAlive(10_000L) { vm.load(hostId) }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { 
                    Column {
                        Text(host?.hostname ?: "主机详情", fontWeight = FontWeight.Bold, style = MaterialTheme.typography.titleMedium, color = MaterialTheme.colorScheme.onSurface)
                        if (host != null) {
                            Text(host?.ip ?: "未知 IP", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    }
                },
                navigationIcon = {
                    IconButton(onClick = { navController.popBackStack() }) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "返回", tint = MaterialTheme.colorScheme.onSurface)
                    }
                },
                actions = {
                    IconButton(onClick = { showAiAssist = true }, enabled = host != null) {
                        Icon(Icons.Default.AutoAwesome, contentDescription = "AI 分析", tint = MaterialTheme.colorScheme.onSurface)
                    }
                    IconButton(onClick = { showCategoryDialog = true }, enabled = host != null) {
                        Icon(Icons.Default.Category, contentDescription = "编辑分组", tint = MaterialTheme.colorScheme.onSurface)
                    }
                    IconButton(onClick = { vm.load(hostId) }, enabled = !loading) {
                        if (loading) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp, color = Color(0xFF4F7FFF))
                        else Icon(Icons.Default.Refresh, contentDescription = "刷新主机详情", tint = MaterialTheme.colorScheme.onSurface)
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.background,
                    titleContentColor = MaterialTheme.colorScheme.onSurface
                )
            )
        },
        containerColor = MaterialTheme.colorScheme.background
    ) { padding ->
        when {
            error != null && host == null -> StateBox(message = error ?: "同步错误", modifier.fillMaxSize().padding(padding), onRetry = { vm.load(hostId) })
            loading && host == null -> LoadingBox(Modifier.fillMaxSize().padding(padding))
            host == null -> StateBox("未找到主机", Modifier.fillMaxSize().padding(padding))
            else -> LazyColumn(
                modifier = modifier.fillMaxSize().padding(padding).padding(horizontal = 16.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp),
                contentPadding = PaddingValues(bottom = 24.dp)
            ) {
                if (error != null) {
                    item {
                        Card(colors = CardDefaults.cardColors(containerColor = Color(0xFFF59E0B).copy(alpha = 0.09f))) {
                            Text(
                                "实时数据刷新失败：${error ?: "未知错误"}",
                                color = Color(0xFFF59E0B),
                                style = MaterialTheme.typography.bodySmall,
                                modifier = Modifier.fillMaxWidth().padding(10.dp)
                            )
                        }
                    }
                }
                item {
                    val currentHost = host ?: return@item
                    Row(
                        modifier = Modifier.fillMaxWidth().padding(vertical = 8.dp),
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        StatusDot(if (currentHost.online) Color(0xFF00A86B) else Color(0xFFEF4444), size = 10.dp)
                        StatusPill(if (currentHost.online) "在线" else "离线", if (currentHost.online) Color(0xFF00A86B) else Color(0xFFEF4444))
                        currentHost.category?.let { StatusPill(it.uppercase(), Color(0xFF4F7FFF)) }
                        Spacer(Modifier.weight(1f))
                        Text(
                            "运行时间: ${formatUptime(metrics?.uptime ?: 0)}",
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    }
                }

                if (history.isNotEmpty()) {
                    item {
                        SectionCard(title = "${vmChartRange.label}性能趋势 (点击查看详情)") {
                            Row(Modifier.fillMaxWidth().height(140.dp), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                                val cpuData = history.map { it.cpu_percent }
                                val memData = history.map { it.mem_percent }
                                val dskData = history.map { it.disk_percent }
                                val timestamps = history.map { it.timestamp }
                                
                                TrendBox("CPU", cpuData, timestamps, Color(0xFF4F7FFF), modifier = Modifier.weight(1f).clickable { showChartMetric = HostChartMetric.CPU }, currentValue = cpuData.lastOrNull())
                                TrendBox("内存", memData, timestamps, Color(0xFF00A86B), modifier = Modifier.weight(1f).clickable { showChartMetric = HostChartMetric.MEM }, currentValue = memData.lastOrNull())
                                TrendBox("磁盘", dskData, timestamps, Color(0xFFF59E0B), modifier = Modifier.weight(1f).clickable { showChartMetric = HostChartMetric.DISK }, currentValue = dskData.lastOrNull())
                            }
                            val gpuHistory = history.filter { !it.gpus.isNullOrEmpty() }
                            if (gpuHistory.size >= 2) {
                                Spacer(Modifier.height(10.dp))
                                val gpuTimestamps = gpuHistory.map { it.timestamp }
                                val gpuUtil = gpuHistory.map { sample -> sample.gpus.orEmpty().maxOfOrNull { it.util_percent } ?: 0.0 }
                                val gpuMemory = gpuHistory.map { sample -> sample.gpus.orEmpty().maxOfOrNull { it.mem_percent } ?: 0.0 }
                                Row(Modifier.fillMaxWidth().height(140.dp), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                                    TrendBox(
                                        "GPU 峰值", gpuUtil, gpuTimestamps, Color(0xFF8B5CF6),
                                        modifier = Modifier.weight(1f).clickable {
                                            showChartMetric = HostChartMetric.GPU_UTIL
                                        }, currentValue = gpuUtil.lastOrNull()
                                    )
                                    TrendBox(
                                        "GPU 显存", gpuMemory, gpuTimestamps, Color(0xFF06B6D4),
                                        modifier = Modifier.weight(1f).clickable {
                                            showChartMetric = HostChartMetric.GPU_MEM
                                        }, currentValue = gpuMemory.lastOrNull()
                                    )
                                }
                            }
                        }
                    }
                }

                item {
                    SectionCard(title = "系统资源") {
                        val m = metrics
                        if (m != null) {
                            Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
                                val cpu = m.cpu_percent.toFloat()
                                val mem = m.mem_percent.toFloat()
                                val disk = m.disk_percent.toFloat()
                                val cpuDetail = if (m.cpu_cores > 0) {
                                    val usedCores = m.cpu_percent / 100.0 * m.cpu_cores
                                    "%.1f / %d 核".format(usedCores, m.cpu_cores)
                                } else if (m.load1 > 0) {
                                    "负载 %.2f".format(m.load1)
                                } else null
                                val memDetail = if (m.mem_total > 0) {
                                    val usedMem = if (m.mem_used > 0) m.mem_used else (m.mem_percent / 100.0 * m.mem_total).toLong()
                                    "${formatBytes(usedMem)} / ${formatBytes(m.mem_total)}"
                                } else null
                                val diskDetail = if (m.disk_total > 0) {
                                    val usedDisk = if (m.disk_used > 0) m.disk_used else (m.disk_percent / 100.0 * m.disk_total).toLong()
                                    "${formatBytes(usedDisk)} / ${formatBytes(m.disk_total)}"
                                } else null
                                MetricBar("CPU 负载", "%.1f%%".format(cpu), cpu / 100f, thresholdColor(cpu), detailText = cpuDetail)
                                if ((m.cpu_iowait ?: 0.0) > 0.1) {
                                    InfoRow("CPU I/O 等待", "%.1f%%".format(m.cpu_iowait ?: 0.0))
                                }
                                MetricBar("内存使用", "%.1f%%".format(mem), mem / 100f, thresholdColor(mem), detailText = memDetail)
                                m.swap_percent?.takeIf { it > 0.1 }?.let { swapPercent ->
                                    val swapDetail = if (m.swap_total > 0) {
                                        val usedSwap = if (m.swap_used > 0) m.swap_used else (swapPercent / 100.0 * m.swap_total).toLong()
                                        "${formatBytes(usedSwap)} / ${formatBytes(m.swap_total)}"
                                    } else null
                                    MetricBar("Swap 使用", "%.1f%%".format(swapPercent), (swapPercent / 100f).toFloat(), MaterialTheme.colorScheme.onSurfaceVariant, detailText = swapDetail)
                                }
                                MetricBar("磁盘使用", "%.1f%%".format(disk), disk / 100f, thresholdColor(disk), detailText = diskDetail)
                                
                                HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant, thickness = 0.5.dp)
                                
                                InfoRow("系统平均负载", "%.2f / %.2f / %.2f".format(m.load1, m.load5, m.load15))
                                InfoRow("进程总数", "${m.proc_count}")
                                InfoRow("网络发送", formatRate(m.net_sent_rate))
                                InfoRow("网络接收", formatRate(m.net_recv_rate))
                                InfoRow("磁盘读取", formatRate(m.disk_read_rate))
                                InfoRow("磁盘写入", formatRate(m.disk_write_rate))
                                InfoRow("磁盘 I/O 利用率", "%.1f%%".format(m.disk_io_util_percent))
                            }
                        } else {
                            Text("暂未收到实时指标数据", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    }
                }

                val gpus = metrics?.gpus
                if (!gpus.isNullOrEmpty()) {
                    item {
                        SectionCard(title = "GPU 加速卡 · ${gpus.size} 块") {
                            Column(verticalArrangement = Arrangement.spacedBy(14.dp)) {
                                gpus.forEachIndexed { index, gpu ->
                                    if (index > 0) HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant, thickness = 0.5.dp)
                                    Row(verticalAlignment = Alignment.CenterVertically) {
                                        Column(Modifier.weight(1f)) {
                                            Text(gpu.name, color = MaterialTheme.colorScheme.onSurface, fontWeight = FontWeight.Bold, maxLines = 1)
                                            Text("GPU ${index + 1}", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
                                        }
                                        if (gpu.temp > 0) StatusPill("%.0f°C".format(gpu.temp), thresholdColor(gpu.temp.toFloat()))
                                    }
                                    MetricBar("计算利用率", "%.1f%%".format(gpu.util_percent), (gpu.util_percent / 100).toFloat(), thresholdColor(gpu.util_percent.toFloat()))
                                    MetricBar("显存利用率", "%.1f%%".format(gpu.mem_percent), (gpu.mem_percent / 100).toFloat(), thresholdColor(gpu.mem_percent.toFloat()))
                                    if (gpu.mem_total > 0) {
                                        InfoRow("显存用量", "${formatBytes(gpu.mem_used)} / ${formatBytes(gpu.mem_total)}")
                                        if (gpu.mem_free > 0) InfoRow("显存可用", formatBytes(gpu.mem_free))
                                    }
                                    InfoRow("核心温度", if (gpu.temp > 0) "%.1f°C".format(gpu.temp) else "暂未采集")
                                }
                            }
                        }
                    }
                }

                // Top Processes
                val procs = metrics?.processes
                if (!procs.isNullOrEmpty()) {
                    item {
                        SectionCard(title = "TOP 进程") {
                            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                                    Text("进程名", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.weight(1f))
                                    Text("CPU", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.width(50.dp), textAlign = androidx.compose.ui.text.style.TextAlign.End)
                                    Text("内存", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.width(50.dp), textAlign = androidx.compose.ui.text.style.TextAlign.End)
                                }
                                procs.take(10).forEach { p ->
                                    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                                        Text(p.name, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurface, maxLines = 1, overflow = androidx.compose.ui.text.style.TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
                                        Text("%.1f%%".format(p.cpu), style = MaterialTheme.typography.bodySmall, color = Color(0xFF4F7FFF), modifier = Modifier.width(50.dp), textAlign = androidx.compose.ui.text.style.TextAlign.End)
                                        Text("%.1f%%".format(p.mem), style = MaterialTheme.typography.bodySmall, color = Color(0xFF00A86B), modifier = Modifier.width(50.dp), textAlign = androidx.compose.ui.text.style.TextAlign.End)
                                    }
                                }
                            }
                        }
                    }
                }

                item {
                    SectionCard(title = "硬件与环境") {
                        InfoRow("操作系统 / 平台", "${host?.os ?: "-"} (${host?.platform ?: "-"})")
                        InfoRow("架构", host?.arch ?: "-")
                        InfoRow("内核版本", host?.kernel ?: "-")
                        InfoRow("初次发现", formatDate(host?.first_seen ?: 0))
                    }
                }

                val disks = metrics?.disks
                if (!disks.isNullOrEmpty()) {
                    item {
                        SectionCard(title = "磁盘分区") {
                            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                                disks.forEach { vol ->
                                    DiskInfoItem(vol)
                                }
                            }
                        }
                    }
                }

                item {
                    PrimaryButton(
                        text = if (host?.online == true) "打开远程终端" else "主机离线，终端不可用",
                        enabled = host?.online == true,
                        onClick = {
                            navController.navigate(Routes.terminal(hostId))
                        }
                    )
                }
            }
        }

        // 交互式全屏图表对话框（从实时 history 派生数据，切换时间范围后自动刷新）
        showChartMetric?.let { metric ->
            val (liveValues, liveTimestamps) = remember(history, metric) { metric.extractValues(history) }
            FullscreenChartDialog(
                title = metric.title,
                data = liveValues,
                timestamps = liveTimestamps,
                color = metric.color,
                yLabel = "%",
                timeRange = vmChartRange,
                isLoading = historyLoading,
                onTimeRangeChange = {
                    vm.setChartTimeRange(it)
                },
                onDismiss = { showChartMetric = null }
            )
        }

        if (showCategoryDialog) {
            var catText by remember(host?.category) { mutableStateOf(host?.category ?: "") }
            AlertDialog(
                onDismissRequest = { showCategoryDialog = false },
                title = { Text("编辑分组") },
                text = {
                    Column {
                        Text("给主机设置分组标签（如 公司专用/家庭专用），留空则清除分组。", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        Spacer(Modifier.height(10.dp))
                        OutlinedTextField(value = catText, onValueChange = { catText = it }, singleLine = true, label = { Text("分组") }, modifier = Modifier.fillMaxWidth())
                    }
                },
                confirmButton = { Button(onClick = { vm.setCategory(catText); showCategoryDialog = false }) { Text("保存") } },
                dismissButton = { TextButton(onClick = { showCategoryDialog = false }) { Text("取消") } }
            )
        }

        if (showAiAssist) {
            val h = host
            if (h != null) {
            val m = metrics
            val ctx = buildString {
                appendLine("主机：${h.hostname}（id=${h.id} IP=${h.ip}）")
                appendLine("在线：${h.online} 分组：${h.category ?: "-"}")
                if (m != null) {
                    appendLine("CPU ${"%.1f".format(m.cpu_percent)}%  内存 ${"%.1f".format(m.mem_percent)}%  磁盘 ${"%.1f".format(m.disk_percent)}%")
                    appendLine("负载 load1=${"%.2f".format(m.load1)} load5=${"%.2f".format(m.load5)} load15=${"%.2f".format(m.load15)}")
                }
                if (history.isNotEmpty()) {
                    val cpu = history.map { it.cpu_percent }
                    val mem = history.map { it.mem_percent }
                    appendLine("近期趋势样本 ${history.size}：CPU 峰值 ${"%.1f".format(cpu.maxOrNull() ?: 0.0)}% 内存峰值 ${"%.1f".format(mem.maxOrNull() ?: 0.0)}%")
                }
            }
            AiAssistSheet(
                title = "AI 主机分析 · ${h.hostname}",
                task = "chart_analysis",
                context = ctx,
                onDismiss = { showAiAssist = false }
            )
            }
        }
    }
}

@Composable
fun FullscreenChartDialog(
    title: String,
    data: List<Double>,
    timestamps: List<Long> = emptyList(),
    color: Color = Color(0xFF4F7FFF),
    yLabel: String = "%",
    pointDetails: List<String> = emptyList(),
    timeRange: ChartTimeRange? = null,
    isLoading: Boolean = false,
    onTimeRangeChange: ((ChartTimeRange) -> Unit)? = null,
    onDismiss: () -> Unit
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false),
        modifier = Modifier.fillMaxSize(),
        confirmButton = {},
        title = null,
        text = {
            Column(Modifier.fillMaxSize().padding(bottom = 8.dp)) {
                Row(Modifier.fillMaxWidth(), Arrangement.SpaceBetween, Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        Text(title, style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface)
                        Text("原生时间序列 · 可精确查看任意采样点", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    IconButton(onClick = onDismiss) {
                        Icon(Icons.Default.Close, contentDescription = "关闭", tint = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                }
                Spacer(Modifier.height(10.dp))
                if (timeRange != null && onTimeRangeChange != null) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        TimeRangeSelector(timeRange, onTimeRangeChange, modifier = Modifier.weight(1f))
                        if (isLoading) {
                            CircularProgressIndicator(
                                modifier = Modifier.size(18.dp).padding(start = 6.dp),
                                strokeWidth = 2.dp,
                                color = Color(0xFF4F7FFF)
                            )
                        }
                    }
                    Spacer(Modifier.height(8.dp))
                }
                InteractiveTimeSeriesChart(
                    values = data,
                    timestamps = timestamps,
                    color = color,
                    unit = yLabel,
                    pointDetails = pointDetails,
                    modifier = Modifier.weight(1f).fillMaxWidth()
                )
                Text(
                    chartRangeDescription(timestamps, data.size),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }
        },
        containerColor = MaterialTheme.colorScheme.surface
    )
}

@Composable
fun TrendBox(
    label: String,
    data: List<Double>,
    timestamps: List<Long>,
    color: Color,
    modifier: Modifier = Modifier,
    currentValue: Double? = null
) {
    Column(modifier.padding(4.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(label, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
            Spacer(Modifier.weight(1f))
            // 显示当前值
            if (currentValue != null) {
                Text("%.1f%%".format(currentValue), style = MaterialTheme.typography.labelSmall, color = color, fontWeight = FontWeight.Bold, fontSize = 10.sp)
                Spacer(Modifier.width(4.dp))
            }
            Icon(Icons.Default.Fullscreen, contentDescription = null, tint = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.5f), modifier = Modifier.size(10.dp))
        }
        Spacer(Modifier.height(4.dp))
        Sparkline(
            data = data,
            timestamps = timestamps,
            color = color,
            modifier = Modifier.fillMaxSize().padding(vertical = 4.dp)
        )
    }
}

private fun chartRangeDescription(timestamps: List<Long>, pointCount: Int): String {
    val valid = timestamps.filter { it > 0 }
    if (valid.size < 2) return "共 $pointCount 个采样点。"
    val start = valid.minOrNull() ?: return "共 $pointCount 个采样点。"
    val end = valid.maxOrNull() ?: return "共 $pointCount 个采样点。"
    val formatter = java.text.SimpleDateFormat("MM-dd HH:mm:ss", java.util.Locale.getDefault())
    fun format(timestamp: Long): String {
        val millis = if (timestamp > 1_000_000_000_000L) timestamp else timestamp * 1000
        return formatter.format(java.util.Date(millis))
    }
    return "${format(start)} 至 ${format(end)} · $pointCount 个采样点"
}

@Composable
fun DiskInfoItem(vol: DiskInfo) {
    val pctVal = vol.percent.toFloat()
    val pct = (pctVal / 100f).coerceIn(0f, 1f)
    val color = thresholdColor(pctVal)
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Row(Modifier.fillMaxWidth(), Arrangement.SpaceBetween) {
            Text(vol.path, style = MaterialTheme.typography.labelSmall, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface)
            Text("%.1f%%".format(pctVal), style = MaterialTheme.typography.labelSmall, fontWeight = FontWeight.Bold, color = color)
        }
        Box(
            modifier = Modifier.fillMaxWidth().height(4.dp).clip(RoundedCornerShape(2.dp)).background(MaterialTheme.colorScheme.outlineVariant)
        ) {
            Box(modifier = Modifier.fillMaxWidth(pct).fillMaxHeight().background(color))
        }
        Row(Modifier.fillMaxWidth(), Arrangement.SpaceBetween) {
            Text("已用: ${formatBytes(vol.used)}", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
            Text("总量: ${formatBytes(vol.total)}", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
        }
        if ((vol.inodes_percent ?: 0.0) > 0.1) {
            Text("Inodes 使用率: %.1f%%".format(vol.inodes_percent), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
        }
    }
}

private fun formatUptime(seconds: Long): String {
    val safeSeconds = seconds.coerceAtLeast(0)
    val d = safeSeconds / 86400
    val h = (safeSeconds % 86400) / 3600
    val m = (safeSeconds % 3600) / 60
    return if (d > 0) "${d}天 ${h}时" else "${h}时 ${m}分"
}

private fun formatDate(ts: Long): String {
    if (ts == 0L) return "-"
    val sdf = java.text.SimpleDateFormat("yyyy-MM-dd HH:mm", java.util.Locale.getDefault())
    val millis = if (ts > 1_000_000_000_000L) ts else ts * 1000
    return sdf.format(java.util.Date(millis))
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TimeRangeSelector(
    selected: ChartTimeRange,
    onRangeChange: (ChartTimeRange) -> Unit,
    modifier: Modifier = Modifier
) {
    Row(
        modifier = modifier.horizontalScroll(rememberScrollState()),
        horizontalArrangement = Arrangement.spacedBy(6.dp)
    ) {
        ChartTimeRange.entries.forEach { range ->
            FilterChip(
                selected = range == selected,
                onClick = { onRangeChange(range) },
                label = { Text(range.label, fontSize = 11.sp) },
                colors = FilterChipDefaults.filterChipColors(
                    containerColor = Color.Transparent,
                    labelColor = MaterialTheme.colorScheme.onSurfaceVariant,
                    selectedContainerColor = Color(0xFF4F7FFF).copy(alpha = 0.2f),
                    selectedLabelColor = Color(0xFF4F7FFF)
                ),
                border = FilterChipDefaults.filterChipBorder(
                    borderColor = MaterialTheme.colorScheme.outlineVariant,
                    selectedBorderColor = Color(0xFF4F7FFF).copy(alpha = 0.6f),
                    enabled = true,
                    selected = range == selected
                ),
                modifier = Modifier.height(28.dp)
            )
        }
    }
}
