package com.aiops.monitor.ui

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.background
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.Build
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.Speed
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarDefaults
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.platform.LocalDensity
import androidx.navigation.NavHostController
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import android.net.Uri
import androidx.compose.ui.platform.LocalContext
import androidx.activity.ComponentActivity
import androidx.lifecycle.ViewModelProvider
import com.aiops.monitor.MainActivity
import com.aiops.monitor.data.ApiClient
import com.aiops.monitor.data.push.PushService
import com.aiops.monitor.data.store.SettingsStore
import com.aiops.monitor.ui.viewmodel.AiCopilotViewModel
import com.aiops.monitor.ui.screens.AlertsScreen
import com.aiops.monitor.ui.screens.AiCopilotScreen
import com.aiops.monitor.ui.screens.DashboardScreen
import com.aiops.monitor.ui.screens.HostDetailScreen
import com.aiops.monitor.ui.screens.LoginScreen
import com.aiops.monitor.ui.screens.InfraHubScreen
import com.aiops.monitor.ui.screens.DashboardListScreen
import com.aiops.monitor.ui.screens.DashboardViewScreen
import com.aiops.monitor.ui.screens.OperationsScreen
import com.aiops.monitor.ui.screens.ExecutionDetailScreen
import com.aiops.monitor.ui.screens.SettingsScreen
import com.aiops.monitor.ui.screens.InstallAgentScreen
import com.aiops.monitor.ui.screens.CostManagementScreen
import com.aiops.monitor.ui.screens.UsersAdminScreen
import com.aiops.monitor.ui.screens.ActivityAuditScreen
import com.aiops.monitor.ui.screens.KnowledgeScreen
import com.aiops.monitor.ui.screens.TerminalScreen
import com.aiops.monitor.data.push.Notifications
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import com.aiops.monitor.ui.screens.TerminalPasswordScreen
import com.aiops.monitor.ui.screens.MessagesScreen
import com.aiops.monitor.ui.screens.DuplicatesScreen
import com.aiops.monitor.ui.screens.TerminalSessionsScreen
import com.aiops.monitor.ui.screens.TerminalReplayScreen
import com.aiops.monitor.ui.screens.AlertExtrasScreen
import kotlinx.coroutines.launch

object Routes {
    const val SETTINGS = "settings"
    const val LOGIN = "login"
    const val DASHBOARD = "dashboard"
    const val ALERTS = "alerts"
    const val ALERTS_FILTERED = "alerts/{level}"
    const val AI_COPILOT = "ai_copilot"
    const val AI_DIAGNOSE = "ai_copilot/diagnose/{hostId}"
    const val HOST_DETAIL = "host/{hostId}"
    const val MONITOR = "monitor"
    const val INFRA_TAB = "infra/tab/{tab}?res={res}"
    const val DASHBOARDS = "dashboards"
    const val DASHBOARD_VIEW = "dashboards/{id}"
    const val HARDWARE = "hardware"
    const val NETFLOW = "netflow"
    const val OPERATIONS = "operations"
    const val OPERATIONS_TAB = "operations/tab/{tab}"
    const val EXECUTION_DETAIL = "operations/execution/{executionId}"
    const val TERMINAL = "terminal/{hostId}"
    const val TERMINAL_PASSWORD = "terminal_password"
    const val INSTALL_AGENT = "install_agent"
    const val COST_MANAGEMENT = "cost_management"
    const val MESSAGES = "messages"
    const val DUPLICATES = "duplicates"
    const val TERMINAL_SESSIONS = "terminal_sessions"
    const val TERMINAL_REPLAY = "terminal_replay/{id}"
    const val ALERT_EXTRAS = "alert_extras"
    const val USERS_ADMIN = "users_admin"
    const val ACTIVITY_AUDIT = "activity_audit"
    const val KNOWLEDGE = "knowledge"

    fun hostDetail(id: String) = "host/${Uri.encode(id)}"
    fun terminal(id: String) = "terminal/${Uri.encode(id)}"
    fun diagnose(id: String) = "ai_copilot/diagnose/${Uri.encode(id)}"
    fun alerts(level: String) = "alerts/${Uri.encode(level)}"
    fun operationsTab(tab: Int) = "operations/tab/$tab"
    fun infraTab(tab: Int, res: Int = 0) = "infra/tab/$tab?res=$res"
    fun dashboardView(id: String) = "dashboards/${Uri.encode(id)}"
    fun terminalReplay(id: String) = "terminal_replay/${Uri.encode(id)}"
    fun executionDetail(id: Long) = "operations/execution/$id"
}

@Composable
fun AIOpsApp(settingsStore: SettingsStore, initialDeepLink: String? = null) {
    val navController = rememberNavController()
    val scope = rememberCoroutineScope()

    // 用可空的“整个设置对象”区分 DataStore 尚未读取与确实没有服务器配置。
    val persistedSettings by settingsStore.appSettings.collectAsState(initial = null)
    val baseUrl = persistedSettings?.baseUrl
    val sessionCookie = persistedSettings?.sessionCookie
    var configurationReady by remember { mutableStateOf(false) }
    var configurationError by remember { mutableStateOf<String?>(null) }
    var sessionProbeDone by remember { mutableStateOf(false) }
    var sessionValid by remember { mutableStateOf(false) }
    var pendingDeepLink by remember { mutableStateOf(initialDeepLink) }
    val density = LocalDensity.current
    val imeVisible = WindowInsets.ime.getBottom(density) > 0
    val context = LocalContext.current

    // Activity 新 Intent（通知深链）时更新
    LaunchedEffect(Unit) {
        val act = context as? MainActivity
        act?.deepLinkRoute?.collect { route ->
            if (!route.isNullOrBlank()) pendingDeepLink = route
        }
    }

    LaunchedEffect(baseUrl, sessionCookie, persistedSettings != null) {
        if (persistedSettings == null) return@LaunchedEffect
        configurationReady = false
        sessionProbeDone = false
        configurationError = null
        sessionValid = false
        if (baseUrl.isNullOrBlank()) {
            ApiClient.clearSession()
            configurationReady = true
            sessionProbeDone = true
            return@LaunchedEffect
        }
        try {
            ApiClient.init(baseUrl)
            if (sessionCookie.isNullOrBlank() || ApiClient.isUnauthorizedPending()) {
                ApiClient.clearSession()
                configurationReady = true
                sessionProbeDone = true
                return@LaunchedEffect
            }
            ApiClient.injectCookieHeader(sessionCookie)
            configurationReady = true
            // 冷启动：用 /me 校验 cookie，无效则清会话，避免闪进 Dashboard
            val ok = withContext(Dispatchers.IO) {
                runCatching { ApiClient.api.me(); ApiClient.isSessionAlive() }.getOrDefault(false)
            }
            if (!ok) {
                settingsStore.clearSessionCookie()
                ApiClient.clearSession()
                sessionValid = false
            } else {
                sessionValid = true
            }
        } catch (error: IllegalArgumentException) {
            configurationError = error.message ?: "服务器地址无效"
            configurationReady = true
        } catch (_: Exception) {
            settingsStore.clearSessionCookie()
            ApiClient.clearSession()
            configurationReady = true
            sessionValid = false
        }
        sessionProbeDone = true
    }

    // 推送前台服务随会话生命周期起停
    val loggedIn = configurationReady && sessionProbeDone && configurationError == null &&
        sessionValid && ApiClient.isSessionAlive() && !ApiClient.isUnauthorizedPending()
    LaunchedEffect(loggedIn) {
        if (loggedIn) {
            (context as? MainActivity)?.requestNotificationPermissionIfNeeded()
            PushService.start(context)
        } else {
            PushService.stop(context)
        }
    }

    // 深链：登录就绪后跳一次
    LaunchedEffect(loggedIn, pendingDeepLink, sessionProbeDone) {
        val route = pendingDeepLink ?: return@LaunchedEffect
        if (!loggedIn || !sessionProbeDone) return@LaunchedEffect
        pendingDeepLink = null
        val dest = when (route) {
            Notifications.ROUTE_ALERTS -> Routes.ALERTS
            Notifications.ROUTE_OPERATIONS -> Routes.OPERATIONS
            else -> route
        }
        runCatching {
            navController.navigate(dest) { launchSingleTop = true }
        }
    }

    // 会话失效：先清持久化 cookie，再清内存，单次导航回登录，杜绝 401 风暴闪退。
    DisposableEffect(Unit) {
        ApiClient.onUnauthorized = {
            scope.launch {
                try {
                    settingsStore.clearSessionCookie()
                    ApiClient.clearSession()
                    PushService.stop(context)
                    (context as? ComponentActivity)?.let { act ->
                        runCatching {
                            ViewModelProvider(act)[AiCopilotViewModel::class.java].onSessionEnded()
                        }
                    }
                    val current = navController.currentDestination?.route
                    if (current != Routes.LOGIN) {
                        navController.navigate(Routes.LOGIN) {
                            val popId = runCatching { navController.graph.id }.getOrNull()
                            if (popId != null) popUpTo(popId) { inclusive = true }
                            else popUpTo(0) { inclusive = true }
                            launchSingleTop = true
                        }
                    }
                } catch (e: Exception) {
                    android.util.Log.w("AIOpsApp", "auth-death navigate failed: ${e.message}")
                }
            }
        }
        onDispose { ApiClient.onUnauthorized = null }
    }

    if (persistedSettings == null || !configurationReady || !sessionProbeDone) {
        Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            CircularProgressIndicator()
        }
        return
    }

    Scaffold(
        containerColor = androidx.compose.material3.MaterialTheme.colorScheme.background,
        // 系统状态栏由各页面 TopAppBar 处理，底部手势区由 NavigationBar 处理。
        // 外层只负责底部主导航高度，避免嵌套 Scaffold 重复计算安全区。
        contentWindowInsets = WindowInsets(0, 0, 0, 0),
        bottomBar = {
            val route = currentRoute(navController)
            val showBottomBar = route in listOf(
                Routes.DASHBOARD, Routes.MONITOR, Routes.INFRA_TAB, Routes.ALERTS, Routes.ALERTS_FILTERED,
                Routes.OPERATIONS, Routes.OPERATIONS_TAB, Routes.AI_COPILOT, Routes.AI_DIAGNOSE
            )
            // 输入场景下隐藏主导航，避免底栏被 adjustResize 顶到键盘上方，
            // 让搜索、AI 对话和表单保留完整输入空间。
            if (showBottomBar && !imeVisible) {
                BottomNavBar(navController)
            }
        }
    ) { padding ->
        NavHost(
            navController = navController,
            startDestination = when {
                // 无服务器地址时仍进登录页：由右下角隐藏入口配置/切换环境（不再强制进设置页）
                sessionValid && !sessionCookie.isNullOrBlank() && configurationError == null -> Routes.DASHBOARD
                else -> Routes.LOGIN
            },
            modifier = Modifier.fillMaxSize().background(androidx.compose.material3.MaterialTheme.colorScheme.background)
        ) {
            composable(Routes.LOGIN) { LoginScreen(navController, settingsStore) }
            composable(Routes.DASHBOARDS) {
                DashboardListScreen(navController, Modifier.fillMaxSize().padding(padding))
            }
            composable(
                Routes.DASHBOARD_VIEW,
                arguments = listOf(navArgument("id") { type = NavType.StringType })
            ) {
                DashboardViewScreen(it.arguments?.getString("id") ?: "", navController, Modifier.fillMaxSize().padding(padding))
            }
            composable(Routes.DASHBOARD) {
                DashboardScreen(navController, settingsStore, Modifier.fillMaxSize().padding(padding))
            }
            composable(Routes.MONITOR) {
                InfraHubScreen(navController, Modifier.fillMaxSize().padding(padding), initialTab = 0)
            }
            composable(
                Routes.INFRA_TAB,
                arguments = listOf(
                    navArgument("tab") { type = NavType.IntType },
                    navArgument("res") { type = NavType.IntType; defaultValue = 0 }
                )
            ) {
                InfraHubScreen(
                    navController,
                    Modifier.fillMaxSize().padding(padding),
                    initialTab = it.arguments?.getInt("tab") ?: 0,
                    initialResSeg = it.arguments?.getInt("res") ?: 0,
                    showBack = true
                )
            }
            composable(Routes.ALERTS) {
                AlertsScreen(navController, Modifier.fillMaxSize().padding(padding))
            }
            composable(
                Routes.ALERTS_FILTERED,
                arguments = listOf(navArgument("level") { type = NavType.StringType })
            ) {
                AlertsScreen(
                    navController,
                    Modifier.fillMaxSize().padding(padding),
                    initialLevel = it.arguments?.getString("level") ?: "all",
                    showBack = true
                )
            }
            composable(Routes.OPERATIONS) {
                OperationsScreen(navController, Modifier.fillMaxSize().padding(padding))
            }
            composable(
                Routes.OPERATIONS_TAB,
                arguments = listOf(navArgument("tab") { type = NavType.IntType })
            ) {
                OperationsScreen(
                    navController,
                    Modifier.fillMaxSize().padding(padding),
                    initialTab = it.arguments?.getInt("tab") ?: 0,
                    showBack = true
                )
            }
            composable(
                Routes.EXECUTION_DETAIL,
                arguments = listOf(navArgument("executionId") { type = NavType.LongType })
            ) {
                ExecutionDetailScreen(
                    navController = navController,
                    modifier = Modifier.fillMaxSize().padding(padding)
                )
            }
            composable(Routes.AI_COPILOT) {
                AiCopilotScreen(Modifier.fillMaxSize().padding(padding))
            }
            composable(
                Routes.AI_DIAGNOSE,
                arguments = listOf(navArgument("hostId") { type = NavType.StringType })
            ) {
                AiCopilotScreen(
                    modifier = Modifier.fillMaxSize().padding(padding),
                    initialHostId = it.arguments?.getString("hostId")
                )
            }
            composable(Routes.SETTINGS) {
                SettingsScreen(
                    settingsStore = settingsStore,
                    navController = navController,
                    modifier = Modifier.fillMaxSize().padding(padding),
                    initialError = configurationError
                )
            }
            composable(Routes.INSTALL_AGENT) {
                InstallAgentScreen(
                    navController = navController,
                    modifier = Modifier.fillMaxSize().padding(padding)
                )
            }
            composable(Routes.COST_MANAGEMENT) {
                CostManagementScreen(
                    navController = navController,
                    modifier = Modifier.fillMaxSize().padding(padding)
                )
            }
            composable(Routes.USERS_ADMIN) {
                UsersAdminScreen(navController, Modifier.fillMaxSize().padding(padding))
            }
            composable(Routes.ACTIVITY_AUDIT) {
                ActivityAuditScreen(navController, Modifier.fillMaxSize().padding(padding))
            }
            composable(Routes.KNOWLEDGE) {
                KnowledgeScreen(navController, Modifier.fillMaxSize().padding(padding))
            }
            composable(
                Routes.HOST_DETAIL,
                arguments = listOf(navArgument("hostId") { type = NavType.StringType })
            ) {
                HostDetailScreen(
                    hostId = it.arguments?.getString("hostId") ?: "",
                    navController = navController,
                    modifier = Modifier.fillMaxSize().padding(padding)
                )
            }
            composable(
                Routes.TERMINAL,
                arguments = listOf(navArgument("hostId") { type = NavType.StringType })
            ) {
                TerminalScreen(
                    hostId = it.arguments?.getString("hostId") ?: "",
                    navController = navController,
                    modifier = Modifier.fillMaxSize().padding(padding)
                )
            }
            composable(Routes.TERMINAL_PASSWORD) {
                TerminalPasswordScreen(
                    navController = navController,
                    modifier = Modifier.fillMaxSize().padding(padding)
                )
            }
            composable(Routes.MESSAGES) {
                MessagesScreen(navController, Modifier.fillMaxSize().padding(padding))
            }
            composable(Routes.DUPLICATES) {
                DuplicatesScreen(navController, Modifier.fillMaxSize().padding(padding))
            }
            composable(Routes.TERMINAL_SESSIONS) {
                TerminalSessionsScreen(navController, Modifier.fillMaxSize().padding(padding))
            }
            composable(
                Routes.TERMINAL_REPLAY,
                arguments = listOf(navArgument("id") { type = NavType.StringType })
            ) {
                TerminalReplayScreen(
                    sessionId = it.arguments?.getString("id") ?: "",
                    navController = navController,
                    modifier = Modifier.fillMaxSize().padding(padding)
                )
            }
            composable(Routes.ALERT_EXTRAS) {
                AlertExtrasScreen(navController, Modifier.fillMaxSize().padding(padding))
            }
        }
    }
}

@Composable
fun currentRoute(navController: NavHostController): String? {
    val entry by navController.currentBackStackEntryAsState()
    return entry?.destination?.route
}

@Composable
fun BottomNavBar(navController: NavHostController) {
    val route = currentRoute(navController)
    val destinations = listOf(
        BottomDestination(Routes.DASHBOARD, "总览", Icons.Filled.Home),
        BottomDestination(Routes.MONITOR, "监控", Icons.Filled.Speed),
        BottomDestination(Routes.ALERTS, "告警", Icons.Filled.Warning),
        BottomDestination(Routes.OPERATIONS, "运维", Icons.Filled.Build),
        BottomDestination(Routes.AI_COPILOT, "AI", Icons.Filled.AutoAwesome)
    )
    NavigationBar(
        modifier = Modifier.fillMaxWidth(),
        containerColor = androidx.compose.material3.MaterialTheme.colorScheme.surface,
        contentColor = androidx.compose.material3.MaterialTheme.colorScheme.onSurface,
        tonalElevation = 3.dp,
        windowInsets = NavigationBarDefaults.windowInsets
    ) {
        destinations.forEach { destination ->
            val selected = route == destination.route ||
                (destination.route == Routes.MONITOR && route == Routes.INFRA_TAB) ||
                (destination.route == Routes.ALERTS && route == Routes.ALERTS_FILTERED) ||
                (destination.route == Routes.OPERATIONS && route == Routes.OPERATIONS_TAB) ||
                (destination.route == Routes.AI_COPILOT && route == Routes.AI_DIAGNOSE)
            NavigationBarItem(
                selected = selected,
                onClick = { navController.navigateBottom(destination.route) },
                icon = {
                    Icon(
                        destination.icon,
                        contentDescription = destination.label,
                        modifier = Modifier.size(23.dp)
                    )
                },
                label = { Text(destination.label, fontSize = 11.sp, maxLines = 1) },
                alwaysShowLabel = true,
                colors = NavigationBarItemDefaults.colors(
                    selectedIconColor = androidx.compose.material3.MaterialTheme.colorScheme.primary,
                    selectedTextColor = androidx.compose.material3.MaterialTheme.colorScheme.primary,
                    indicatorColor = androidx.compose.material3.MaterialTheme.colorScheme.primaryContainer,
                    unselectedIconColor = androidx.compose.material3.MaterialTheme.colorScheme.onSurfaceVariant,
                    unselectedTextColor = androidx.compose.material3.MaterialTheme.colorScheme.onSurfaceVariant
                )
            )
        }
    }
}

private data class BottomDestination(
    val route: String,
    val label: String,
    val icon: ImageVector
)

private fun NavHostController.navigateBottom(route: String) {
    navigate(route) {
        popUpTo(Routes.DASHBOARD) { saveState = true }
        launchSingleTop = true
        restoreState = true
    }
}
