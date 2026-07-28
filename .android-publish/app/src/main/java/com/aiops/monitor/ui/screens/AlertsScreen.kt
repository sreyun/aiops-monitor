package com.aiops.monitor.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.VolumeOff
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavHostController
import com.aiops.monitor.data.ApiClient
import com.aiops.monitor.data.SessionTicker
import com.aiops.monitor.data.models.Alert
import com.aiops.monitor.ui.Routes
import com.aiops.monitor.ui.components.*
import com.aiops.monitor.ui.viewmodel.AlertsViewModel
import com.aiops.monitor.ui.viewmodel.aiCopilotViewModel
import kotlinx.coroutines.delay

private val AlertBlue = AppBlue
private val AlertGreen = AppGreen
private val AlertOrange = AppOrange
private val AlertRed = AppRed

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AlertsScreen(
    navController: NavHostController,
    modifier: Modifier = Modifier,
    initialLevel: String = "all",
    showBack: Boolean = false
) {
    val vm: AlertsViewModel = viewModel()
    // Activity 级 AI 助手 VM：与 AiCopilotScreen 共享同一实例，告警上下文才传得过去。
    val aiVm = aiCopilotViewModel()
    val alerts by vm.alerts.collectAsState()
    val loading by vm.loading.collectAsState()
    val error by vm.error.collectAsState()
    val notice by vm.notice.collectAsState()
    val busyKeys by vm.busyKeys.collectAsState()
    var selectedLevel by remember(initialLevel) {
        mutableStateOf(initialLevel.takeIf { it in setOf("all", "critical", "warning", "active", "resolved") } ?: "all")
    }
    var selectedStatus by remember { mutableStateOf("all") }
    var pendingSilence by remember { mutableStateOf<Alert?>(null) }
    val snackbarHostState = remember { SnackbarHostState() }

    LaunchedEffect(Unit) {
        SessionTicker.pollWhileAlive(15_000L) { vm.load() }
    }

    val criticalCount = alerts.count { it.level.equals("critical", true) }
    val warningCount = alerts.count { it.level.equals("warning", true) }
    val activeCount = alerts.count { it.status.isNullOrBlank() }
    val acknowledgedCount = alerts.count { it.status == "acknowledged" }
    val silencedCount = alerts.count { it.status == "silenced" }
    val resolvedCount = acknowledgedCount + silencedCount

    // 两个筛选维度**同时**生效：严重级别（顶部统计卡）+ 处理状态（状态 chip）。
    // 此前 filteredAlerts 只读 selectedLevel，状态 chip 设了 selectedStatus 却从不参与过滤，
    // 点“待处理/已确认/已静默”毫无反应——这是那个死筛选 bug。
    val filteredAlerts = alerts.filter { alert ->
        val levelOk = when (selectedLevel) {
            "critical" -> alert.level.equals("critical", true)
            "warning" -> alert.level.equals("warning", true)
            "active" -> alert.status.isNullOrBlank()
            "resolved" -> alert.status == "acknowledged" || alert.status == "silenced"
            else -> true // "all"
        }
        val statusOk = when (selectedStatus) {
            "active" -> alert.status.isNullOrBlank()
            "acknowledged" -> alert.status == "acknowledged"
            "silenced" -> alert.status == "silenced"
            else -> true // "all"
        }
        levelOk && statusOk
    }.sortedWith(compareBy<Alert> {
        when (it.status) { null, "" -> 0; "acknowledged" -> 1; "silenced" -> 2; else -> 3 }
    }.thenBy {
        when (it.level.lowercase()) { "critical" -> 0; "warning" -> 1; else -> 2 }
    }.thenByDescending { it.timestamp })

    LaunchedEffect(notice) {
        notice?.let {
            snackbarHostState.showSnackbar(it)
            vm.clearNotice()
        }
    }
    LaunchedEffect(error, alerts.isNotEmpty()) {
        if (alerts.isNotEmpty()) error?.let {
            snackbarHostState.showSnackbar(it)
            vm.clearError()
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        Text("告警中心", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface)
                        Text(
                            when {
                                loading -> "正在同步告警数据…"
                                alerts.isEmpty() -> "所有系统运行正常，无活动告警"
                                else -> "共 ${alerts.size} 条告警 · 15 秒自动刷新"
                            },
                            style = MaterialTheme.typography.bodySmall,
                            color = if (criticalCount > 0) AlertRed else MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    }
                },
                navigationIcon = {
                    if (showBack) IconButton(onClick = { navController.popBackStack() }) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "返回", tint = MaterialTheme.colorScheme.onSurface)
                    }
                },
                actions = {
                    IconButton(onClick = { navController.navigate(Routes.ALERT_EXTRAS) }) {
                        Icon(Icons.Default.History, "历史与治理", tint = MaterialTheme.colorScheme.onSurface)
                    }
                    if (loading) {
                        CircularProgressIndicator(Modifier.size(19.dp).padding(end = 8.dp), strokeWidth = 2.dp, color = AlertBlue)
                    } else {
                        IconButton(onClick = vm::load) { Icon(Icons.Default.Refresh, "刷新告警", tint = MaterialTheme.colorScheme.onSurface) }
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background)
            )
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
        containerColor = MaterialTheme.colorScheme.background
    ) { padding ->
        when {
            error != null && alerts.isEmpty() -> StateBox(message = error ?: "同步错误", modifier = modifier.fillMaxSize().padding(padding), onRetry = { vm.load() })
            loading && alerts.isEmpty() -> LoadingBox(Modifier.fillMaxSize().padding(padding))
            else -> LazyColumn(
                modifier = modifier.fillMaxSize().padding(padding).padding(horizontal = 14.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp),
                contentPadding = PaddingValues(top = 8.dp, bottom = 20.dp)
            ) {
                if (error != null) {
                    item(key = "error") {
                        Card(colors = CardDefaults.cardColors(containerColor = AlertOrange.copy(alpha = 0.09f))) {
                            Text(error ?: "部分操作失败", color = AlertOrange, style = MaterialTheme.typography.bodySmall, modifier = Modifier.fillMaxWidth().padding(10.dp))
                        }
                    }
                }

                // ── 级别筛选（原 2×2 大指标卡 → 一行紧凑 chip，计数与筛选都保留，版面更聚焦告警列表）──
                if (alerts.isNotEmpty()) {
                    item(key = "level-filter") {
                        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                            Text("告警级别", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 11.sp, fontWeight = FontWeight.Medium)
                            Row(
                                Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                                horizontalArrangement = Arrangement.spacedBy(8.dp)
                            ) {
                                listOf(
                                    Triple("all", "全部", alerts.size),
                                    Triple("critical", "严重", criticalCount),
                                    Triple("warning", "警告", warningCount)
                                ).forEach { (key, label, count) ->
                                    FilterChip(
                                        selected = selectedLevel == key,
                                        onClick = { selectedLevel = if (selectedLevel == key) "all" else key; selectedStatus = "all" },
                                        label = { Text("$label $count") },
                                        colors = chipColors()
                                    )
                                }
                            }
                        }
                    }
                }

                // ── 处理状态筛选 ──
                if (alerts.isNotEmpty()) {
                    item(key = "status-filter") {
                        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                            Text("处理状态", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 11.sp, fontWeight = FontWeight.Medium)
                            Row(
                                Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                                horizontalArrangement = Arrangement.spacedBy(8.dp)
                            ) {
                                listOf(
                                    Triple("all", "全部", alerts.size),
                                    Triple("active", "待处理", activeCount),
                                    Triple("acknowledged", "已确认", acknowledgedCount),
                                    Triple("silenced", "已静默", silencedCount)
                                ).forEach { (key, label, count) ->
                                    FilterChip(
                                        selected = selectedStatus == key,
                                        onClick = {
                                            selectedStatus = key
                                            // 顶部卡的 active/resolved 与状态维度会打架（如
                                            // level=active + status=acknowledged 恒为空），
                                            // 选状态时把 level 的状态类快捷清掉，只保留严重级别。
                                            if (selectedLevel == "active" || selectedLevel == "resolved") selectedLevel = "all"
                                        },
                                        label = { Text("$label $count") },
                                        colors = chipColors()
                                    )
                                }
                            }
                        }
                    }
                }

                // ── 列表标题 ──
                if (alerts.isNotEmpty()) {
                    item(key = "list-header") {
                        Row(
                            Modifier.fillMaxWidth(),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(8.dp)
                        ) {
                            Text("告警列表", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface)
                            Spacer(Modifier.weight(1f))
                            Text("显示 ${filteredAlerts.size} / ${alerts.size}", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    }
                }

                // ── 空状态 ──
                if (alerts.isEmpty() && !loading) {
                    item(key = "empty") {
                        Card(
                            colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
                            elevation = CardDefaults.cardElevation(defaultElevation = 0.dp),
                            shape = RoundedCornerShape(14.dp)
                        ) {
                            Column(
                                Modifier.fillMaxWidth().padding(vertical = 48.dp, horizontal = 24.dp),
                                horizontalAlignment = Alignment.CenterHorizontally,
                                verticalArrangement = Arrangement.spacedBy(12.dp)
                            ) {
                                Icon(Icons.Default.CheckCircle, contentDescription = null, modifier = Modifier.size(48.dp), tint = AlertGreen)
                                Text("所有系统运行正常", style = MaterialTheme.typography.titleMedium, color = AlertGreen, fontWeight = FontWeight.Bold)
                                Text("暂无活动告警，监控数据每 15 秒自动刷新", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            }
                        }
                    }
                }

                if (alerts.isNotEmpty() && filteredAlerts.isEmpty()) {
                    item(key = "no-match") {
                        Card(
                            colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
                            elevation = CardDefaults.cardElevation(defaultElevation = 0.dp),
                            shape = RoundedCornerShape(14.dp)
                        ) {
                            Column(
                                Modifier.fillMaxWidth().padding(32.dp),
                                horizontalAlignment = Alignment.CenterHorizontally,
                                verticalArrangement = Arrangement.spacedBy(12.dp)
                            ) {
                                Icon(Icons.Default.Search, contentDescription = null, tint = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.size(36.dp))
                                Text("当前筛选条件下没有告警", color = MaterialTheme.colorScheme.onSurface, fontWeight = FontWeight.Bold)
                                TextButton(onClick = { selectedLevel = "all"; selectedStatus = "all" }) { Text("清除筛选") }
                            }
                        }
                    }
                }

                // ── 告警卡片列表 ──
                // key 必须唯一：同一主机同类型同 scope 可能有多条告警（如同一硬件
                // 目标的多个子部件 scope 相同），只用 alertBusyKey 会撞键直接崩溃。
                // 追加列表下标兜底，保证唯一。
                itemsIndexed(
                    filteredAlerts,
                    key = { index, alert -> "$index#${alertBusyKey(alert)}" }
                ) { _, alert ->
                    AlertCard(
                        alert = alert,
                        busy = alertBusyKey(alert) in busyKeys,
                        onAck = { vm.ack(alert) { vm.load() } },
                        onSilence = { pendingSilence = alert },
                        onDiagnose = {
                            aiVm.setPendingAlertDiagnosis(alert)
                            navController.navigate(Routes.AI_COPILOT) { launchSingleTop = true }
                        }
                    )
                }
            }
        }
    }

    pendingSilence?.let { alert ->
        val busy = alertBusyKey(alert) in busyKeys
        AlertDialog(
            onDismissRequest = { if (!busy) pendingSilence = null },
            containerColor = MaterialTheme.colorScheme.surface,
            shape = RoundedCornerShape(16.dp),
            title = { Text("确认静默告警？", fontSize = 16.sp, fontWeight = FontWeight.Bold) },
            text = {
                Text(
                    "将静默 ${alert.hostname} 的 ${localizeLevel(alert.level)} ${alert.type.uppercase()} 告警。静默期间可能不再收到重复通知，请确认风险已知。",
                    fontSize = 13.sp,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            },
            confirmButton = {
                Button(
                    onClick = {
                        vm.silence(alert) {
                            pendingSilence = null
                            vm.load()
                        }
                    },
                    enabled = !busy,
                    colors = ButtonDefaults.buttonColors(containerColor = AlertOrange),
                    shape = RoundedCornerShape(10.dp)
                ) {
                    if (busy) CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp, color = MaterialTheme.colorScheme.onSurface)
                    else Text("确认静默", fontSize = 13.sp)
                }
            },
            dismissButton = { TextButton(onClick = { pendingSilence = null }, enabled = !busy) { Text("取消", fontSize = 13.sp) } }
        )
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// 概览统计行（2×2 StatCard）
// ─────────────────────────────────────────────────────────────────────────────

@Composable
private fun AlertSummaryRow(
    critical: Int,
    warning: Int,
    active: Int,
    resolved: Int,
    selectedLevel: String,
    onLevelClick: (String) -> Unit
) {
    Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            StatCard("严重告警", "$critical", AlertRed, Modifier.weight(1f), selected = selectedLevel == "critical") { onLevelClick("critical") }
            StatCard("警告告警", "$warning", AlertOrange, Modifier.weight(1f), selected = selectedLevel == "warning") { onLevelClick("warning") }
        }
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            StatCard("待处理", "$active", if (active > 0) AlertRed else AlertGreen, Modifier.weight(1f), selected = selectedLevel == "active") { onLevelClick("active") }
            StatCard("已处理", "$resolved", AlertBlue, Modifier.weight(1f), selected = selectedLevel == "resolved") { onLevelClick("resolved") }
        }
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// 告警卡片（与 HostCard 风格统一：左侧彩色指示条 + 结构化布局）
// ─────────────────────────────────────────────────────────────────────────────

@Composable
fun AlertCard(alert: Alert, busy: Boolean, onAck: () -> Unit, onSilence: () -> Unit, onDiagnose: () -> Unit) {
    val accent = statusColor(alert.level)
    val borderColor = accent.copy(alpha = 0.15f)

    Card(
        modifier = Modifier.fillMaxWidth().border(1.dp, borderColor, RoundedCornerShape(12.dp)),
        shape = RoundedCornerShape(12.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 0.dp)
    ) {
        Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min)) {
            Box(Modifier.width(4.dp).fillMaxHeight().background(accent))

            Column(Modifier.weight(1f).padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                // 第一行：状态点 + 主机名 + 级别标签 + 状态标签
                Row(verticalAlignment = Alignment.CenterVertically) {
                    StatusDot(accent, size = 10.dp)
                    Spacer(Modifier.width(8.dp))
                    Text(
                        alert.hostname,
                        style = MaterialTheme.typography.titleMedium,
                        fontWeight = FontWeight.Bold,
                        color = MaterialTheme.colorScheme.onSurface,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f)
                    )
                    if (!alert.status.isNullOrBlank()) {
                        StatusPill(localizeAlertStatus(alert.status), if (alert.status == "silenced") AlertOrange else AlertBlue)
                        Spacer(Modifier.width(6.dp))
                    }
                    StatusPill(localizeLevel(alert.level), accent)
                }

                // 第二行：告警消息（突出显示）
                Text(
                    alert.message,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurface.copy(alpha = 0.9f),
                    maxLines = 3,
                    overflow = TextOverflow.Ellipsis,
                    lineHeight = 16.sp
                )

                // 第三行：元数据（类型 · 范围 · IP · 持续时间）
                Row(
                    Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    StatusPill(localizeAlertType(alert.type), AlertBlue)
                    alert.scope?.let {
                        StatusPill(it.uppercase(), MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    alert.ip?.let {
                        Text(it, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
                    }
                    alert.since?.let {
                        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(3.dp)) {
                            Icon(Icons.Default.Schedule, null, Modifier.size(11.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant)
                            Text(formatDuration(it), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
                        }
                    }
                }

                // 操作按钮行
                HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant, thickness = 0.5.dp)
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    // 确认按钮
                    Button(
                        onClick = onAck,
                        enabled = !busy && alert.status != "acknowledged",
                        modifier = Modifier.weight(1f).height(36.dp),
                        contentPadding = PaddingValues(horizontal = 6.dp),
                        shape = RoundedCornerShape(10.dp),
                        colors = ButtonDefaults.buttonColors(
                            containerColor = if (alert.status == "acknowledged") AlertGreen.copy(alpha = 0.15f) else MaterialTheme.colorScheme.outlineVariant.copy(alpha = 0.5f),
                            contentColor = if (alert.status == "acknowledged") AlertGreen else MaterialTheme.colorScheme.onSurface,
                            disabledContainerColor = MaterialTheme.colorScheme.outlineVariant.copy(alpha = 0.2f)
                        )
                    ) {
                        if (busy) CircularProgressIndicator(Modifier.size(14.dp), strokeWidth = 2.dp, color = MaterialTheme.colorScheme.onSurface)
                        else {
                            Icon(if (alert.status == "acknowledged") Icons.Default.CheckCircle else Icons.Default.Check, null, Modifier.size(13.dp))
                            Spacer(Modifier.width(4.dp))
                            Text(if (alert.status == "acknowledged") "已确认" else "确认", fontSize = 11.sp, fontWeight = FontWeight.Bold)
                        }
                    }

                    // 静默按钮
                    OutlinedButton(
                        onClick = onSilence,
                        enabled = !busy && alert.status != "silenced",
                        modifier = Modifier.weight(1f).height(36.dp),
                        contentPadding = PaddingValues(horizontal = 6.dp),
                        shape = RoundedCornerShape(10.dp),
                        border = androidx.compose.foundation.BorderStroke(1.dp, if (alert.status == "silenced") AlertOrange.copy(alpha = 0.4f) else MaterialTheme.colorScheme.outlineVariant),
                        colors = ButtonDefaults.outlinedButtonColors(
                            contentColor = if (alert.status == "silenced") AlertOrange else MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    ) {
                        Icon(Icons.AutoMirrored.Filled.VolumeOff, null, Modifier.size(13.dp))
                        Spacer(Modifier.width(4.dp))
                        Text(if (alert.status == "silenced") "已静默" else "静默", fontSize = 11.sp, fontWeight = FontWeight.Bold)
                    }

                    // AI 诊断：带完整告警上下文跳到 AI 助手（复用已建好的 diagnoseAlert 桥接）
                    OutlinedButton(
                        onClick = onDiagnose,
                        enabled = !busy,
                        modifier = Modifier.weight(1f).height(36.dp),
                        contentPadding = PaddingValues(horizontal = 6.dp),
                        shape = RoundedCornerShape(10.dp),
                        border = androidx.compose.foundation.BorderStroke(1.dp, AlertBlue.copy(alpha = 0.5f)),
                        colors = ButtonDefaults.outlinedButtonColors(contentColor = AlertBlue)
                    ) {
                        Icon(Icons.Default.AutoAwesome, null, Modifier.size(13.dp))
                        Spacer(Modifier.width(4.dp))
                        Text("AI 诊断", fontSize = 11.sp, fontWeight = FontWeight.Bold)
                    }
                }
            }
        }
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// 工具函数
// ─────────────────────────────────────────────────────────────────────────────

private fun alertBusyKey(alert: Alert): String =
    "${alert.host_id}|${alert.type}|${alert.scope.orEmpty()}"

@Composable
fun MetadataItem(text: String, color: Color) {
    Text(text, style = MaterialTheme.typography.labelSmall, color = color, fontWeight = FontWeight.Bold, fontSize = 10.sp, letterSpacing = 0.5.sp)
}

private fun localizeLevel(level: String): String = when (level.lowercase()) {
    "critical" -> "严重"
    "warning" -> "警告"
    else -> level.uppercase()
}

private fun localizeAlertStatus(status: String): String = when (status) {
    "acknowledged" -> "已确认"
    "silenced" -> "已静默"
    else -> "待处理"
}

private fun localizeAlertType(type: String): String = when (type.lowercase()) {
    "cpu" -> "CPU"
    "memory", "mem" -> "内存"
    "disk" -> "磁盘"
    "offline" -> "离线"
    "load" -> "负载"
    "gpu" -> "GPU"
    "process", "proc" -> "进程"
    "diskio" -> "磁盘IO"
    "iops" -> "IOPS"
    "conn" -> "连接"
    "check" -> "探测"
    "api" -> "API"
    "task" -> "任务"
    "forward" -> "转发"
    else -> type.replaceFirstChar { it.uppercase() }
}

private fun formatDuration(since: Long): String {
    val seconds = (System.currentTimeMillis() / 1000) - since
    if (seconds < 0) return "刚刚"
    val m = seconds / 60
    val h = m / 60
    val d = h / 24
    return when {
        d > 0 -> "${d}天${h % 24}时"
        h > 0 -> "${h}时${m % 60}分"
        m > 0 -> "${m}分"
        else -> "${seconds}秒"
    }
}
