@file:OptIn(ExperimentalFoundationApi::class)
package com.aiops.monitor.ui.screens

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.ui.draw.clip
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Memory
import androidx.compose.material.icons.filled.Lan
import androidx.compose.material.icons.filled.Language
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.Dvr
import androidx.compose.material.icons.filled.Insights
import androidx.compose.material.icons.filled.Dashboard
import androidx.compose.material.icons.filled.Build
import androidx.compose.material.icons.filled.SwapHoriz
import androidx.compose.material.icons.filled.Policy
import androidx.compose.material.icons.filled.NotificationsActive
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Terminal
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.WbSunny
import androidx.compose.material.icons.filled.Cloud
import androidx.compose.material.icons.filled.Grain
import androidx.compose.material.icons.filled.Air
import androidx.compose.material.icons.filled.AcUnit
import androidx.compose.material.icons.filled.Thunderstorm
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material.icons.automirrored.filled.ManageSearch
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.automirrored.filled.Logout
import androidx.compose.material.icons.filled.SwitchAccount
import androidx.compose.material.icons.filled.Security
import androidx.compose.material.icons.filled.DarkMode
import androidx.compose.material.icons.filled.LightMode
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.activity.ComponentActivity
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavHostController
import com.aiops.monitor.data.models.Alert
import com.aiops.monitor.data.models.Host
import com.aiops.monitor.data.models.Incident
import com.aiops.monitor.data.models.Summary
import com.aiops.monitor.data.ApiClient
import com.aiops.monitor.data.SessionTicker
import com.aiops.monitor.data.push.PushService
import com.aiops.monitor.data.store.SettingsStore
import com.aiops.monitor.data.store.ThemeMode
import com.aiops.monitor.ui.Routes
import com.aiops.monitor.ui.components.*
import com.aiops.monitor.ui.viewmodel.AiCopilotViewModel
import com.aiops.monitor.ui.viewmodel.HostsViewModel
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

private val AccentBlue = AppBlue
private val OkGreen = AppGreen
private val WarnOrange = AppOrange
private val CritRed = AppRed

private fun sevColor(s: String): Color = when (s.lowercase()) {
    "critical", "严重" -> CritRed
    "warning", "警告" -> WarnOrange
    else -> AccentBlue
}

private fun levelLabel(level: String): String = when (level.lowercase()) {
    "critical" -> "严重"; "warning" -> "警告"; else -> level
}

private fun relTime(ts: Long): String {
    if (ts <= 0) return ""
    val millis = if (ts > 1_000_000_000_000L) ts else ts * 1000
    val diff = (System.currentTimeMillis() - millis) / 1000
    return when {
        diff < 60 -> "刚刚"
        diff < 3600 -> "${diff / 60}分钟前"
        diff < 86400 -> "${diff / 3600}小时前"
        else -> "${diff / 86400}天前"
    }
}

@OptIn(ExperimentalMaterial3Api::class, ExperimentalFoundationApi::class)
@Composable
fun DashboardScreen(navController: NavHostController, settingsStore: SettingsStore, modifier: Modifier = Modifier) {
    val vm: HostsViewModel = viewModel()
    val hosts by vm.hosts.collectAsState()
    val summary by vm.summary.collectAsState()
    val loading by vm.loading.collectAsState()
    val error by vm.error.collectAsState()
    val me by vm.me.collectAsState()
    val weatherResp by vm.weather.collectAsState()
    val sreOverview by vm.sreOverview.collectAsState()
    val openIncidents by vm.openIncidents.collectAsState()
    val activeAlerts by vm.activeAlerts.collectAsState()
    val themeMode by settingsStore.themeMode.collectAsState(initial = ThemeMode.DARK)
    val scope = rememberCoroutineScope()
    val context = LocalContext.current

    var showAccountMenu by remember { mutableStateOf(false) }
    var pendingAccountAction by remember { mutableStateOf<AccountAction?>(null) }
    var accountActionBusy by remember { mutableStateOf(false) }

    fun finishSession() {
        if (accountActionBusy) return
        accountActionBusy = true
        scope.launch {
            try {
                try { ApiClient.api.logout() } catch (_: Exception) { }
                // 与 auth-death 路径一致：先清持久化，再清内存并卸掉整栈，避免返回键回到已登出页。
                settingsStore.clearSessionCookie()
                ApiClient.clearSession()
                PushService.stop(context)
                runCatching {
                    ViewModelProvider(context as ComponentActivity)[AiCopilotViewModel::class.java]
                        .onSessionEnded()
                }
                navController.navigate(Routes.LOGIN) {
                    val popId = runCatching { navController.graph.id }.getOrNull()
                    if (popId != null) popUpTo(popId) { inclusive = true }
                    else popUpTo(0) { inclusive = true }
                    launchSingleTop = true
                }
            } finally {
                pendingAccountAction = null
                accountActionBusy = false
            }
        }
    }

    LaunchedEffect(Unit) {
        SessionTicker.pollWhileAlive(10_000L) { vm.load() }
    }

    // 活跃告警：剔除已确认/已静默/已解决
    val active = remember(activeAlerts) {
        activeAlerts.filter { it.status != "acknowledged" && it.status != "silenced" && it.status != "resolved" }
    }

    Scaffold(
        modifier = modifier,
        topBar = {
            TopAppBar(
                title = {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text("SRE 驾驶舱", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface)
                        if (loading) {
                            Spacer(Modifier.width(8.dp))
                            CircularProgressIndicator(Modifier.size(14.dp), strokeWidth = 2.dp, color = AccentBlue)
                        }
                    }
                },
                actions = {
                    IconButton(onClick = { vm.load() }, modifier = Modifier.size(36.dp)) {
                        Icon(Icons.Default.Refresh, contentDescription = "刷新", tint = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.size(18.dp))
                    }
                    Box {
                        IconButton(onClick = { showAccountMenu = true }, enabled = !accountActionBusy, modifier = Modifier.size(36.dp)) {
                            if (accountActionBusy) CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp, color = AccentBlue)
                            else Icon(Icons.Default.Settings, contentDescription = "配置中心与账号", tint = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.size(18.dp))
                        }
                        DropdownMenu(
                            expanded = showAccountMenu,
                            onDismissRequest = { showAccountMenu = false },
                            containerColor = MaterialTheme.colorScheme.surface,
                            shape = RoundedCornerShape(12.dp)
                        ) {
                            Column(Modifier.padding(horizontal = 14.dp, vertical = 10.dp)) {
                                Text(me?.username ?: "当前账号", fontWeight = FontWeight.Bold, fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurface)
                                Text(
                                    "${me?.role ?: "用户"} · ${if (me?.mfa_enabled == true) "MFA 已启用" else "MFA 未启用"}",
                                    style = MaterialTheme.typography.labelSmall,
                                    color = if (me?.mfa_enabled == true) OkGreen else WarnOrange
                                )
                            }
                            HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant, thickness = 0.5.dp)
                            DropdownMenuItem(
                                text = { Text("配置中心", fontSize = 13.sp) },
                                leadingIcon = { Icon(Icons.Default.Settings, null, modifier = Modifier.size(18.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant) },
                                onClick = { showAccountMenu = false; navController.navigate(Routes.SETTINGS) }
                            )
                            DropdownMenuItem(
                                text = { Text("消息中心", fontSize = 13.sp) },
                                leadingIcon = { Icon(Icons.Default.NotificationsActive, null, modifier = Modifier.size(18.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant) },
                                onClick = { showAccountMenu = false; navController.navigate(Routes.MESSAGES) }
                            )
                            DropdownMenuItem(
                                text = { Text("重复主机清理", fontSize = 13.sp) },
                                leadingIcon = { Icon(Icons.Default.ContentCopy, null, modifier = Modifier.size(18.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant) },
                                onClick = { showAccountMenu = false; navController.navigate(Routes.DUPLICATES) }
                            )
                            DropdownMenuItem(
                                text = { Text("终端会话回放", fontSize = 13.sp) },
                                leadingIcon = { Icon(Icons.Default.Terminal, null, modifier = Modifier.size(18.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant) },
                                onClick = { showAccountMenu = false; navController.navigate(Routes.TERMINAL_SESSIONS) }
                            )
                            DropdownMenuItem(
                                text = {
                                    Column {
                                        Text(if (themeMode == ThemeMode.DARK) "深色模式" else "浅色模式", fontSize = 13.sp)
                                        Text("点击切换应用外观", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                    }
                                },
                                leadingIcon = { Icon(if (themeMode == ThemeMode.DARK) Icons.Default.DarkMode else Icons.Default.LightMode, null, modifier = Modifier.size(18.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant) },
                                trailingIcon = { Switch(checked = themeMode == ThemeMode.LIGHT, onCheckedChange = null) },
                                onClick = {
                                    scope.launch { settingsStore.setThemeMode(if (themeMode == ThemeMode.DARK) ThemeMode.LIGHT else ThemeMode.DARK) }
                                }
                            )
                            DropdownMenuItem(
                                text = { Text("切换账号", fontSize = 13.sp) },
                                leadingIcon = { Icon(Icons.Default.SwitchAccount, null, modifier = Modifier.size(18.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant) },
                                onClick = { showAccountMenu = false; pendingAccountAction = AccountAction.Switch }
                            )
                            DropdownMenuItem(
                                text = { Text("退出登录", color = CritRed, fontSize = 13.sp) },
                                leadingIcon = { Icon(Icons.AutoMirrored.Filled.Logout, null, tint = CritRed, modifier = Modifier.size(18.dp)) },
                                onClick = { showAccountMenu = false; pendingAccountAction = AccountAction.Logout }
                            )
                        }
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background)
            )
        },
        containerColor = MaterialTheme.colorScheme.background
    ) { padding ->
        when {
            error != null && hosts.isEmpty() && summary == null ->
                StateBox(message = error ?: "同步错误", modifier = Modifier.fillMaxSize().padding(padding), onRetry = vm::load)
            loading && hosts.isEmpty() && summary == null ->
                LoadingBox(Modifier.fillMaxSize().padding(padding))
            else -> LazyColumn(
                modifier = Modifier.fillMaxSize().padding(padding).padding(horizontal = 14.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
                contentPadding = PaddingValues(top = 8.dp, bottom = 20.dp)
            ) {
                item(key = "weather-hero") {
                    val w = weatherResp?.takeIf { it.ok }?.let {
                        WeatherState(
                            location = it.location, tempC = it.temp_c, text = it.text,
                            todayHigh = it.today_high, todayLow = it.today_low,
                            tomorrowText = it.tomorrow_text, tomorrowHigh = it.tomorrow_high, tomorrowLow = it.tomorrow_low
                        )
                    }
                    WeatherHero(
                        summary, weather = w,
                        onOnlineClick = { navController.navigate(Routes.infraTab(0)) },
                        onCriticalClick = { navController.navigate(Routes.alerts("critical")) },
                        onWarningClick = { navController.navigate(Routes.alerts("warning")) }
                    )
                }

                // 快捷入口：8 宫格 + 用户自定义（路径最短直达底栏进不去/较深的功能）。
                item(key = "quick-access") { QuickAccessGrid(navController, settingsStore) }

                // 待处理事件（SRE 头等大事）
                item(key = "incidents") {
                    val count = sreOverview?.open_incidents ?: openIncidents.size
                    CockpitCard(
                        title = "待处理事件",
                        count = if (count > 0) "$count" else null,
                        countColor = CritRed,
                        actionLabel = if (openIncidents.isNotEmpty()) "全部" else null,
                        onAction = { navController.navigate(Routes.operationsTab(0)) }
                    ) {
                        if (openIncidents.isEmpty()) {
                            GoodStateRow("暂无待处理事件")
                        } else {
                            openIncidents.sortedByDescending { it.created_at }.take(3).forEach { inc ->
                                IncidentRow(inc) { navController.navigate(Routes.operationsTab(0)) }
                            }
                        }
                    }
                }

                // 活跃告警
                item(key = "alerts") {
                    val crit = active.count { it.level.equals("critical", true) }
                    val warn = active.count { it.level.equals("warning", true) }
                    CockpitCard(
                        title = "活跃告警",
                        count = if (active.isNotEmpty()) "严重 $crit · 警告 $warn" else null,
                        countColor = if (crit > 0) CritRed else WarnOrange,
                        actionLabel = if (active.isNotEmpty()) "全部" else null,
                        onAction = { navController.navigate(Routes.alerts("all")) }
                    ) {
                        if (active.isEmpty()) {
                            GoodStateRow("无活跃告警")
                        } else {
                            active.sortedBy { if (it.level.equals("critical", true)) 0 else 1 }.take(3).forEach { al ->
                                AlertRow(al) { navController.navigate(Routes.alerts(al.level.lowercase())) }
                            }
                        }
                    }
                }

                // 容量热点
                item(key = "hotspots") {
                    val offline = hosts.filter { !it.online }
                    val hot = hosts.filter { it.online && it.latest != null }
                        .map { it to worstLoad(it) }
                        .filter { it.second >= 1.0 }
                        .sortedByDescending { it.second }
                        .take(4)
                    CockpitCard(
                        title = "容量热点",
                        count = if (offline.isNotEmpty()) "离线 ${offline.size}" else null,
                        countColor = CritRed,
                        actionLabel = "主机",
                        onAction = { navController.navigate(Routes.infraTab(0)) }
                    ) {
                        if (offline.isEmpty() && hot.isEmpty()) {
                            GoodStateRow("全部主机负载正常")
                        } else {
                            offline.take(2).forEach { h ->
                                HotspotRow(h.hostname, "离线", CritRed, offline = true) { navController.navigate(Routes.hostDetail(h.id)) }
                            }
                            hot.forEach { (h, v) ->
                                val worst = worstMetric(h)
                                HotspotRow(h.hostname, "${worst.first} ${"%.0f".format(v)}%", thresholdColor(v.toFloat())) { navController.navigate(Routes.hostDetail(h.id)) }
                            }
                        }
                    }
                }

                if (error != null) {
                    item(key = "error") {
                        Card(colors = CardDefaults.cardColors(containerColor = WarnOrange.copy(alpha = 0.09f))) {
                            Text(error ?: "部分数据刷新失败", color = WarnOrange, style = MaterialTheme.typography.bodySmall, modifier = Modifier.fillMaxWidth().padding(10.dp))
                        }
                    }
                }
            }
        }
    }

    pendingAccountAction?.let { action ->
        AlertDialog(
            onDismissRequest = { if (!accountActionBusy) pendingAccountAction = null },
            containerColor = MaterialTheme.colorScheme.surface,
            shape = RoundedCornerShape(16.dp),
            icon = { Icon(Icons.Default.Security, null, tint = AccentBlue, modifier = Modifier.size(28.dp)) },
            title = { Text(if (action == AccountAction.Switch) "切换账号？" else "退出登录？", fontSize = 16.sp) },
            text = {
                Text(
                    if (action == AccountAction.Switch)
                        "将注销当前会话并返回登录页。新账号必须重新验证密码；已启用 MFA 的账号还需输入 6 位动态口令。"
                    else
                        "将同时注销服务器会话并清除本机登录凭据。下次登录仍会执行账号密码与 MFA 验证。",
                    fontSize = 13.sp,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            },
            confirmButton = {
                Button(onClick = ::finishSession, enabled = !accountActionBusy, shape = RoundedCornerShape(10.dp)) {
                    if (accountActionBusy) CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp)
                    else Text(if (action == AccountAction.Switch) "切换" else "退出", fontSize = 13.sp)
                }
            },
            dismissButton = { TextButton(onClick = { pendingAccountAction = null }, enabled = !accountActionBusy) { Text("取消", fontSize = 13.sp) } }
        )
    }
}

private enum class AccountAction { Switch, Logout }

private fun worstLoad(h: Host): Double {
    val l = h.latest ?: return 0.0
    return maxOf(l.cpu_percent, l.mem_percent, l.disk_percent, l.gpus.orEmpty().maxOfOrNull { it.util_percent } ?: 0.0)
}

private fun worstMetric(h: Host): Pair<String, Double> {
    val l = h.latest ?: return "负载" to 0.0
    val metrics = listOf(
        "CPU" to l.cpu_percent, "内存" to l.mem_percent, "磁盘" to l.disk_percent,
        "GPU" to (l.gpus.orEmpty().maxOfOrNull { it.util_percent } ?: 0.0)
    )
    return metrics.maxByOrNull { it.second } ?: ("负载" to 0.0)
}

// ── 驾驶舱区块骨架 ──
@Composable
private fun CockpitCard(
    title: String,
    count: String? = null,
    countColor: Color = AccentBlue,
    actionLabel: String? = null,
    onAction: (() -> Unit)? = null,
    content: @Composable ColumnScope.() -> Unit
) {
    Card(
        Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(14.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 0.dp)
    ) {
        Column(Modifier.padding(10.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(title, fontWeight = FontWeight.Bold, fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurface)
                if (count != null) {
                    Spacer(Modifier.width(6.dp))
                    Box(Modifier.clip(RoundedCornerShape(6.dp)).background(countColor.copy(alpha = 0.14f)).padding(horizontal = 6.dp, vertical = 1.dp)) {
                        Text(count, color = countColor, fontSize = 10.sp, fontWeight = FontWeight.Bold)
                    }
                }
                Spacer(Modifier.weight(1f))
                if (actionLabel != null && onAction != null) {
                    Row(
                        Modifier.clip(RoundedCornerShape(6.dp)).clickable(onClick = onAction).padding(horizontal = 4.dp, vertical = 2.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text(actionLabel, color = AccentBlue, fontSize = 11.sp)
                        Icon(Icons.Default.ChevronRight, null, tint = AccentBlue, modifier = Modifier.size(14.dp))
                    }
                }
            }
            content()
        }
    }
}

@Composable
private fun GoodStateRow(text: String) {
    Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(vertical = 1.dp)) {
        Icon(Icons.Default.CheckCircle, null, tint = OkGreen, modifier = Modifier.size(14.dp))
        Spacer(Modifier.width(6.dp))
        Text(text, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
private fun IncidentRow(inc: Incident, onClick: () -> Unit) {
    Row(
        Modifier.fillMaxWidth().clip(RoundedCornerShape(6.dp)).clickable(onClick = onClick).padding(vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        StatusDot(sevColor(inc.severity), 7.dp)
        Spacer(Modifier.width(8.dp))
        Column(Modifier.weight(1f)) {
            Text(inc.title, fontSize = 12.sp, fontWeight = FontWeight.Medium, color = MaterialTheme.colorScheme.onSurface, maxLines = 1, overflow = TextOverflow.Ellipsis)
            val sub = listOfNotNull(inc.hostname?.takeIf { it.isNotBlank() }, inc.source.takeIf { it.isNotBlank() }).joinToString(" · ")
            if (sub.isNotBlank()) Text(sub, fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
        Text(relTime(inc.created_at), fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
private fun AlertRow(al: Alert, onClick: () -> Unit) {
    Row(
        Modifier.fillMaxWidth().clip(RoundedCornerShape(6.dp)).clickable(onClick = onClick).padding(vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        StatusPill(levelLabel(al.level), sevColor(al.level))
        Spacer(Modifier.width(8.dp))
        Column(Modifier.weight(1f)) {
            Text("${al.hostname} · ${al.type.uppercase()}", fontSize = 12.sp, fontWeight = FontWeight.Medium, color = MaterialTheme.colorScheme.onSurface, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Text(al.message, fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
        Text(relTime(al.since ?: 0), fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
private fun HotspotRow(hostname: String, badge: String, color: Color, offline: Boolean = false, onClick: () -> Unit) {
    Row(
        Modifier.fillMaxWidth().clip(RoundedCornerShape(6.dp)).clickable(onClick = onClick).padding(vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        StatusDot(if (offline) CritRed else OkGreen, 7.dp)
        Spacer(Modifier.width(8.dp))
        Text(hostname, fontSize = 12.sp, fontWeight = FontWeight.Medium, color = MaterialTheme.colorScheme.onSurface, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
        StatusPill(badge, color)
    }
}

// ── 快捷入口（8 宫格 + 用户自定义） ──
private data class QuickModule(val key: String, val label: String, val icon: ImageVector, val tint: Color, val route: String)

// 可选模块目录：覆盖底栏进不去/较深的功能，用户从中挑选最多 8 个呈现在总览。
private val ALL_MODULES = listOf(
    // 资源 Tab(index 1) 内含二级 [物理机=res0 / 虚拟机=res1]；网络=2、拨测=3
    QuickModule("hardware", "硬件监控", Icons.Filled.Memory, AccentBlue, Routes.infraTab(1, 0)),
    QuickModule("hyperv", "虚拟机", Icons.Filled.Dvr, Color(0xFF8B5CF6), Routes.infraTab(1, 1)),
    QuickModule("dashboards", "仪表盘", Icons.Filled.Insights, Color(0xFF06B6D4), Routes.DASHBOARDS),
    QuickModule("netflow", "网络流量", Icons.Filled.Lan, OkGreen, Routes.infraTab(2)),
    QuickModule("probe", "拨测监控", Icons.Filled.Language, Color(0xFF06B6D4), Routes.infraTab(3)),
    QuickModule("hosts", "主机监控", Icons.Filled.Dns, Color(0xFF0EA5E9), Routes.infraTab(0)),
    QuickModule("logs", "日志检索", Icons.AutoMirrored.Filled.ManageSearch, CritRed, Routes.operationsTab(1)),
    QuickModule("incidents", "SRE 事件", Icons.Filled.Dashboard, Color(0xFF8B5CF6), Routes.operationsTab(0)),
    QuickModule("playbooks", "运维编排", Icons.Filled.Build, Color(0xFF06B6D4), Routes.operationsTab(2)),
    QuickModule("forwards", "端口转发", Icons.Filled.SwapHoriz, AccentBlue, Routes.operationsTab(3)),
    QuickModule("governance", "治理审计", Icons.Filled.Policy, WarnOrange, Routes.operationsTab(4)),
    QuickModule("alerts", "告警中心", Icons.Filled.NotificationsActive, WarnOrange, Routes.alerts("all")),
    QuickModule("ai", "AI 助手", Icons.Filled.AutoAwesome, AccentBlue, Routes.AI_COPILOT),
)
private val DEFAULT_MODULE_KEYS = listOf("hardware", "hyperv", "netflow", "probe", "hosts", "logs", "playbooks", "ai")
private const val MAX_MODULES = 8

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun QuickAccessGrid(navController: NavHostController, settingsStore: SettingsStore) {
    val scope = rememberCoroutineScope()
    val savedKeys by settingsStore.quickModules.collectAsState(initial = emptyList())
    var showEdit by remember { mutableStateOf(false) }

    val keys = savedKeys.ifEmpty { DEFAULT_MODULE_KEYS }
    val modules = keys.mapNotNull { k -> ALL_MODULES.find { it.key == k } }.take(MAX_MODULES)
    val rows = modules.chunked(4)

    Card(
        Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(14.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 0.dp)
    ) {
        Column(Modifier.padding(vertical = 10.dp, horizontal = 6.dp)) {
            Row(Modifier.fillMaxWidth().padding(horizontal = 6.dp).padding(bottom = 4.dp), verticalAlignment = Alignment.CenterVertically) {
                Text("快捷入口", fontWeight = FontWeight.Bold, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurface)
                Spacer(Modifier.weight(1f))
                IconButton(
                    onClick = { showEdit = true },
                    modifier = Modifier.size(24.dp)
                ) {
                    Icon(Icons.Default.Add, "自定义快捷入口", tint = AccentBlue, modifier = Modifier.size(16.dp))
                }
            }
            rows.forEachIndexed { idx, row ->
                Row(Modifier.fillMaxWidth()) {
                    row.forEach { m -> QuickTile(m, Modifier.weight(1f)) { navController.navigate(m.route) } }
                    repeat(4 - row.size) { Spacer(Modifier.weight(1f)) }
                }
                if (idx < rows.lastIndex) Spacer(Modifier.height(10.dp))
            }
        }
    }

    if (showEdit) {
        ModuleEditSheet(
            current = keys,
            onDismiss = { showEdit = false },
            onSave = { sel -> scope.launch { settingsStore.setQuickModules(sel) }; showEdit = false }
        )
    }
}

@Composable
private fun QuickTile(module: QuickModule, modifier: Modifier = Modifier, onClick: () -> Unit) {
    Column(
        modifier.clip(RoundedCornerShape(10.dp)).clickable(onClick = onClick).padding(vertical = 3.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(5.dp)
    ) {
        Box(Modifier.size(42.dp).clip(RoundedCornerShape(12.dp)).background(module.tint.copy(alpha = 0.14f)), contentAlignment = Alignment.Center) {
            Icon(module.icon, module.label, tint = module.tint, modifier = Modifier.size(21.dp))
        }
        Text(module.label, fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurface, maxLines = 1)
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ModuleEditSheet(current: List<String>, onDismiss: () -> Unit, onSave: (List<String>) -> Unit) {
    var selected by remember { mutableStateOf(current.filter { k -> ALL_MODULES.any { it.key == k } }) }
    ModalBottomSheet(onDismissRequest = onDismiss, containerColor = MaterialTheme.colorScheme.surface, shape = RoundedCornerShape(topStart = 18.dp, topEnd = 18.dp)) {
        Column(
            Modifier.fillMaxWidth().padding(horizontal = 16.dp).padding(bottom = 24.dp).verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(4.dp)
        ) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("自定义快捷入口", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f), color = MaterialTheme.colorScheme.onSurface)
                Text("${selected.size}/$MAX_MODULES", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 12.sp)
            }
            Text("勾选要在总览显示的模块，最多 $MAX_MODULES 个", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            Spacer(Modifier.height(4.dp))
            ALL_MODULES.forEach { m ->
                val isSel = m.key in selected
                val atCap = selected.size >= MAX_MODULES && !isSel
                Row(
                    Modifier.fillMaxWidth().clip(RoundedCornerShape(10.dp)).clickable(enabled = !atCap) {
                        selected = if (isSel) selected - m.key else selected + m.key
                    }.padding(vertical = 6.dp, horizontal = 6.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Box(Modifier.size(32.dp).clip(RoundedCornerShape(8.dp)).background(m.tint.copy(alpha = if (atCap) 0.06f else 0.14f)), contentAlignment = Alignment.Center) {
                        Icon(m.icon, m.label, tint = if (atCap) m.tint.copy(alpha = 0.4f) else m.tint, modifier = Modifier.size(16.dp))
                    }
                    Spacer(Modifier.width(10.dp))
                    Text(m.label, Modifier.weight(1f), color = if (atCap) MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.onSurface, fontSize = 13.sp)
                    Checkbox(checked = isSel, onCheckedChange = null, enabled = !atCap)
                }
            }
            Spacer(Modifier.height(4.dp))
            Button(onClick = { onSave(selected) }, modifier = Modifier.fillMaxWidth(), enabled = selected.isNotEmpty(), shape = RoundedCornerShape(10.dp)) {
                Text("保存 (${selected.size})", fontSize = 13.sp)
            }
        }
    }
}

private fun formatDashboardTime(timestampMillis: Long): String =
    SimpleDateFormat("HH:mm:ss", Locale.getDefault()).format(Date(timestampMillis))

// ── 天气/欢迎英雄头（保留：问候 + 天气 + 健康脉搏一行） ──
data class WeatherState(
    val location: String = "",
    val tempC: Int = 0,
    val text: String = "",
    val todayHigh: Int = 0,
    val todayLow: Int = 0,
    val tomorrowText: String = "",
    val tomorrowHigh: Int = 0,
    val tomorrowLow: Int = 0,
)

@Composable
private fun WeatherHero(
    summary: Summary?, weather: WeatherState?,
    onOnlineClick: () -> Unit = {},
    onCriticalClick: () -> Unit = {},
    onWarningClick: () -> Unit = {},
) {
    val cal = java.util.Calendar.getInstance()
    val hour = cal.get(java.util.Calendar.HOUR_OF_DAY)
    val greeting = when (hour) { in 5..10 -> "早上好"; in 11..13 -> "中午好"; in 14..18 -> "下午好"; else -> "晚上好" }
    val dateStr = SimpleDateFormat("M月d日 EEEE", Locale.CHINA).format(Date())

    Box(
        Modifier.fillMaxWidth().clip(RoundedCornerShape(16.dp))
            .background(androidx.compose.ui.graphics.Brush.linearGradient(listOf(AccentBlue, Color(0xFF6AA1FF), Color(0xFF67C7F0))))
            .padding(horizontal = 14.dp, vertical = 12.dp)
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            // 顶部：问候（左上）+ 天气图标为背景的温度 & 地点（右上）
            Row(Modifier.fillMaxWidth(), Arrangement.SpaceBetween, Alignment.Top) {
                Column {
                    Text(greeting, color = Color.White, fontWeight = FontWeight.Bold, fontSize = 17.sp)
                    Text(dateStr, color = Color.White.copy(alpha = 0.8f), fontSize = 11.sp)
                }
                if (weather != null) {
                    // 图标作为整个天气区域的半透明背景，温度与地点文字居中叠加
                    Box(contentAlignment = Alignment.Center) {
                        Icon(
                            imageVector = weatherIconFor(weather.text),
                            contentDescription = weather.text,
                            tint = Color.White.copy(alpha = 0.12f),
                            modifier = Modifier.size(58.dp)
                        )
                        Column(horizontalAlignment = Alignment.CenterHorizontally) {
                            Text(
                                "${weather.tempC}°",
                                color = Color.White,
                                fontWeight = FontWeight.Bold,
                                fontSize = 28.sp
                            )
                            Text(
                                "${weather.location} · ${weather.text}",
                                color = Color.White.copy(alpha = 0.85f),
                                fontSize = 10.sp
                            )
                        }
                    }
                }
            }
            // 底部：统计数据行
            if (summary != null) {
                Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    HeroStat("在线", "${summary.online_hosts}/${summary.total_hosts}", onClick = onOnlineClick)
                    HeroStat("严重", "${summary.critical_alerts}", onClick = onCriticalClick)
                    HeroStat("警告", "${summary.warning_alerts}", onClick = onWarningClick)
                }
            }
        }
    }
}

/**
 * 根据天气描述文本返回对应的 Material 图标。
 * 匹配优先级：从高到低；未知天气默认返回太阳图标。
 */
private fun weatherIconFor(text: String): ImageVector {
    val t = text.lowercase()
    return when {
        t.contains("雷") || t.contains("thunder") -> Icons.Filled.Thunderstorm
        t.contains("雪") || t.contains("snow") -> Icons.Filled.AcUnit
        t.contains("雨") || t.contains("rain") || t.contains("shower") -> Icons.Filled.Grain
        t.contains("雾") || t.contains("霾") || t.contains("fog") || t.contains("mist") || t.contains("haze") -> Icons.Filled.Air
        t.contains("阴") || t.contains("overcast") -> Icons.Filled.Cloud
        t.contains("云") || t.contains("cloud") || t.contains("多云") -> Icons.Filled.Cloud
        t.contains("晴") || t.contains("clear") || t.contains("sunny") -> Icons.Filled.WbSunny
        else -> Icons.Filled.WbSunny
    }
}

@Composable
private fun HeroStat(label: String, value: String, wide: Boolean = false, onClick: (() -> Unit)? = null) {
    val mod = if (onClick != null)
        Modifier.clip(RoundedCornerShape(6.dp)).clickable(onClick = onClick).padding(horizontal = 3.dp, vertical = 1.dp)
    else Modifier
    Column(mod) {
        Text(value, color = Color.White, fontWeight = FontWeight.Bold, fontSize = if (wide) 12.sp else 14.sp)
        Text(label, color = Color.White.copy(alpha = 0.75f), fontSize = 9.sp)
    }
}
