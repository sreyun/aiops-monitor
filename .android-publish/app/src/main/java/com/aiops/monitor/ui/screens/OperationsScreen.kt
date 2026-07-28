@file:OptIn(ExperimentalFoundationApi::class)
package com.aiops.monitor.ui.screens

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Lan
import androidx.compose.material.icons.filled.Public
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.BugReport
import androidx.compose.material.icons.filled.History
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Terminal
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.ThumbDown
import androidx.compose.material.icons.filled.ThumbUp
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.ExtendedFloatingActionButton
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Tab
import androidx.compose.material3.TabRow
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
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
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.DialogProperties
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavHostController
import com.aiops.monitor.data.models.Incident
import com.aiops.monitor.data.models.InspectionReport
import com.aiops.monitor.data.models.LogSearchResponse
import com.aiops.monitor.data.models.Playbook
import com.aiops.monitor.data.models.PlaybookExecution
import com.aiops.monitor.data.models.PortForwardRule
import com.aiops.monitor.data.models.HTTPProxyConfig
import com.aiops.monitor.data.models.PortForwardCreateRequest
import com.aiops.monitor.data.models.PortForwardEditRequest
import com.aiops.monitor.data.models.Host
import com.aiops.monitor.data.models.SreOverview
import com.aiops.monitor.data.models.StoredLog
import com.aiops.monitor.data.models.SloStatus
import com.aiops.monitor.data.models.ActivityLogEntry
import com.aiops.monitor.data.models.Ticket
import com.aiops.monitor.data.models.RemediationRun
import com.aiops.monitor.ui.Routes
import com.aiops.monitor.ui.components.AnimatedDotsText
import com.aiops.monitor.ui.components.LoadingBox
import com.aiops.monitor.ui.components.StatusDot
import com.aiops.monitor.ui.components.StatusPill
import com.aiops.monitor.ui.components.MarkdownText
import com.aiops.monitor.ui.components.AppBlue
import com.aiops.monitor.ui.components.AppGreen
import com.aiops.monitor.ui.components.AppOrange
import com.aiops.monitor.ui.components.AppRed
import com.aiops.monitor.ui.viewmodel.OperationsViewModel
import com.aiops.monitor.ui.viewmodel.IncidentDiagnosisUiState
import com.aiops.monitor.data.ApiClient
import com.aiops.monitor.data.SessionTicker
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

private val OpsBlue = AppBlue
private val OpsGreen = AppGreen
private val OpsOrange = AppOrange
private val OpsRed = AppRed

@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class, ExperimentalFoundationApi::class)
@Composable
fun OperationsScreen(navController: NavHostController, modifier: Modifier = Modifier, initialTab: Int = 0, showBack: Boolean = false) {
    val vm: OperationsViewModel = viewModel()
    val overview by vm.overview.collectAsState()
    val incidents by vm.incidents.collectAsState()
    val logs by vm.logs.collectAsState()
    val playbooks by vm.playbooks.collectAsState()
    val executions by vm.executions.collectAsState()
    val executionToOpen by vm.executionToOpen.collectAsState()
    val slos by vm.slos.collectAsState()
    val tickets by vm.tickets.collectAsState()
    val remediationRuns by vm.remediationRuns.collectAsState()
    val loading by vm.loading.collectAsState()
    val logsLoading by vm.logsLoading.collectAsState()
    val busyIds by vm.busyIds.collectAsState()
    val error by vm.error.collectAsState()
    val notice by vm.notice.collectAsState()
    val diagnosis by vm.diagnosis.collectAsState()
    val incidentDiagnosis by vm.incidentDiagnosis.collectAsState()
    val terminalAuditLogs by vm.terminalAuditLogs.collectAsState()
    val terminalSessionsLoading by vm.terminalSessionsLoading.collectAsState()
    val portForwards by vm.portForwards.collectAsState()
    val portForwardsLoading by vm.portForwardsLoading.collectAsState()
    val hosts by vm.hosts.collectAsState()
    val httpProxies by vm.httpProxies.collectAsState()

    // initialTab 让首页宫格能直达指定标签（0 事件/SRE中枢，1 日志检索）。
    var selectedTab by remember(initialTab) { mutableIntStateOf(initialTab) }
    var govTab by remember { mutableIntStateOf(0) }
    var showCreateIncident by remember { mutableStateOf(false) }
    var selectedIncident by remember { mutableStateOf<Incident?>(null) }
    var selectedTicket by remember { mutableStateOf<Ticket?>(null) }
    var pendingPlaybook by remember { mutableStateOf<Playbook?>(null) }
    var editingPortForward by remember { mutableStateOf<PortForwardRule?>(null) }
    var showCreatePortForward by remember { mutableStateOf(false) }
    var deletingPortForward by remember { mutableStateOf<PortForwardRule?>(null) }
    var showCreateHTTPProxy by remember { mutableStateOf(false) }
    var editingHTTPProxy by remember { mutableStateOf<HTTPProxyConfig?>(null) }
    var deletingHTTPProxy by remember { mutableStateOf<HTTPProxyConfig?>(null) }
    val snackbar = remember { SnackbarHostState() }

    LaunchedEffect(Unit) {
        vm.searchLogs("", "", 360)
        SessionTicker.pollWhileAlive(15_000L) { vm.load() }
    }
    LaunchedEffect(notice) {
        notice?.let {
            snackbar.showSnackbar(it)
            vm.clearNotice()
        }
    }
    LaunchedEffect(executionToOpen) {
        val executionId = executionToOpen ?: return@LaunchedEffect
        val navigation = runCatching {
            navController.navigate(Routes.executionDetail(executionId)) {
                launchSingleTop = true
            }
        }
        vm.consumeExecutionNavigation()
        if (navigation.isFailure) vm.reportExecutionNavigationFailure()
    }

    Scaffold(
        modifier = modifier,
        snackbarHost = { SnackbarHost(snackbar) },
        topBar = {
            TopAppBar(
                title = {
                    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        Text("SRE 运维中心", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface)
                        Text("事件闭环 · 日志检索 · 自动化剧本 · 15 秒刷新", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                },
                navigationIcon = {
                    if (showBack) IconButton(onClick = { navController.popBackStack() }) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "返回", tint = MaterialTheme.colorScheme.onSurface)
                    }
                },
                actions = {
                    if (loading) CircularProgressIndicator(Modifier.size(19.dp), strokeWidth = 2.dp, color = OpsBlue)
                    else IconButton(onClick = vm::load) { Icon(Icons.Default.Refresh, "刷新", tint = MaterialTheme.colorScheme.onSurface) }
                    Spacer(Modifier.width(6.dp))
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background)
            )
        },
        floatingActionButton = {
            if (selectedTab == 0) {
                FloatingActionButton(
                    onClick = { showCreateIncident = true },
                    containerColor = OpsBlue,
                    contentColor = Color.White
                ) {
                    Icon(Icons.Default.Add, contentDescription = "新建事件")
                }
            }
        },
        containerColor = MaterialTheme.colorScheme.background
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding)) {
            OpsOverviewRow(
                overview = overview,
                onIncidentsClick = { selectedTab = 0 },
                onRemediationClick = { selectedTab = 4; govTab = 0 },
                onTicketsClick = { selectedTab = 4; govTab = 2 },
                onSloClick = { selectedTab = 4; govTab = 1 }
            )
            TabRow(
                selectedTabIndex = selectedTab,
                containerColor = MaterialTheme.colorScheme.surfaceVariant,
                contentColor = OpsBlue,
                divider = {}
            ) {
                listOf("事件", "日志", "剧本", "转发", "治理").forEachIndexed { index, title ->
                    Tab(
                        selected = selectedTab == index,
                        onClick = { selectedTab = index },
                        text = { Text(title, fontWeight = FontWeight.Bold) }
                    )
                }
            }
            if (error != null) {
                Text(
                    error ?: "加载失败",
                    color = OpsOrange,
                    fontSize = 11.sp,
                    modifier = Modifier.fillMaxWidth().background(OpsOrange.copy(alpha = 0.08f)).padding(9.dp)
                )
            }
            Box(Modifier.weight(1f)) {
                when (selectedTab) {
                    0 -> IncidentList(
                        incidents = incidents,
                        loading = loading,
                        busyIds = busyIds,
                        onSelect = { selectedIncident = it },
                        onAck = vm::ackIncident,
                        onResolve = vm::resolveIncident,
                        onDiagnose = vm::openIncidentDiagnosis
                    )
                    1 -> LogsPanel(
                        response = logs,
                        logsLoading = logsLoading,
                        busyIds = busyIds,
                        terminalAuditLogs = terminalAuditLogs,
                        terminalSessionsLoading = terminalSessionsLoading,
                        onSearch = vm::searchLogs,
                        onDiagnose = vm::diagnoseLog,
                        onRefreshTerminalSessions = vm::loadTerminalSessions
                    )
                    2 -> PlaybooksPanel(
                        playbooks = playbooks,
                        executions = executions,
                        busyIds = busyIds,
                        onExecute = { pendingPlaybook = it },
                        onExecutionClick = { navController.navigate(Routes.executionDetail(it.id)) },
                        onRefreshExecutions = vm::refreshExecutions
                    )
                    3 -> PortForwardPanel(
                        rules = portForwards,
                        loading = portForwardsLoading,
                        hosts = hosts,
                        httpProxies = httpProxies,
                        busyIds = busyIds,
                        onRefresh = { vm.loadPortForwards() },
                        onCreatePortForward = { showCreatePortForward = true },
                        onEditPortForward = { editingPortForward = it },
                        onDeletePortForward = { deletingPortForward = it },
                        onTogglePortForward = { id, enabled -> vm.togglePortForward(id, enabled) },
                        onCreateHTTPProxy = { showCreateHTTPProxy = true },
                        onEditHTTPProxy = { editingHTTPProxy = it },
                        onDeleteHTTPProxy = { deletingHTTPProxy = it },
                        onToggleHTTPProxy = { id, enabled -> vm.toggleHTTPProxy(id, enabled) }
                    )
                    else -> GovernancePanel(
                        slos = slos,
                        tickets = tickets,
                        remediationRuns = remediationRuns,
                        busyIds = busyIds,
                        govTab = govTab,
                        onGovTabChange = { govTab = it },
                        onDecideRemediation = vm::decideRemediation,
                        onResolveTicket = vm::resolveTicket,
                        onSelectTicket = { selectedTicket = it }
                    )
                }
            }
        }
    }

    if (showCreateIncident) {
        CreateIncidentDialog(
            busy = "incident:create" in busyIds,
            onDismiss = { showCreateIncident = false },
            onCreate = { title, severity ->
                vm.createIncident(title, severity) { success -> if (success) showCreateIncident = false }
            }
        )
    }
    selectedIncident?.let { incident ->
        IncidentDetailDialog(
            incident = incidents.find { it.id == incident.id } ?: incident,
            busy = "incident:${incident.id}" in busyIds,
            onDismiss = { selectedIncident = null },
            onAck = { vm.ackIncident(incident.id) },
            onResolve = { vm.resolveIncident(incident.id) },
            onEscalate = { vm.escalateIncident(incident.id) },
            onDiagnose = {
                selectedIncident = null
                vm.openIncidentDiagnosis(incidents.find { it.id == incident.id } ?: incident)
            }
        )
    }
    pendingPlaybook?.let { playbook ->
        AlertDialog(
            onDismissRequest = { pendingPlaybook = null },
            containerColor = MaterialTheme.colorScheme.surface,
            shape = RoundedCornerShape(16.dp),
            title = { Text("执行自动化剧本？", fontSize = 16.sp, fontWeight = FontWeight.Bold) },
            text = {
                Text("\u201C${playbook.name}\u201D将按 ${playbook.steps.orEmpty().size} 个步骤在在线目标主机执行远程命令。请确认目标范围与命令已在 Web 端审核。", fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            },
            confirmButton = {
                Button(onClick = {
                    pendingPlaybook = null
                    vm.executePlaybook(playbook.id)
                }, shape = RoundedCornerShape(10.dp)) { Text("确认执行", fontSize = 13.sp) }
            },
            dismissButton = { TextButton(onClick = { pendingPlaybook = null }) { Text("取消", fontSize = 13.sp) } }
        )
    }
    diagnosis?.let { report ->
        DiagnosisDialog(report, vm::clearDiagnosis)
    }
    selectedTicket?.let { ticket ->
        TicketDetailDialog(
            ticket = tickets.find { it.id == ticket.id } ?: ticket,
            busy = "ticket:${ticket.id}" in busyIds || "ticket-assign:${ticket.id}" in busyIds || "ticket-comment:${ticket.id}" in busyIds,
            onDismiss = { selectedTicket = null },
            onResolve = { vm.resolveTicket(ticket) },
            onAssign = { assignee -> vm.assignTicket(ticket, assignee) },
            onComment = { text, atts -> vm.commentTicket(ticket.id, text, atts) },
            loadUsers = { vm.loadDirectoryUsers() }
        )
    }
    incidentDiagnosis?.let { state ->
        IncidentDiagnosisDialog(
            state = state,
            onDismiss = vm::closeIncidentDiagnosis,
            onTerminalContextChanged = vm::setIncidentDiagnosisTerminalContext,
            onSend = vm::sendIncidentDiagnosisMessage,
            onFeedback = vm::sendIncidentDiagnosisFeedback
        )
    }
    if (showCreatePortForward) {
        PortForwardFormDialog(
            rule = null,
            hosts = hosts,
            busy = "port-forward:create" in busyIds,
            onDismiss = { showCreatePortForward = false },
            onCreate = { req ->
                vm.createPortForward(req)
                showCreatePortForward = false
            },
            onEdit = { _, _ -> }
        )
    }
    editingPortForward?.let { rule ->
        PortForwardFormDialog(
            rule = rule,
            hosts = hosts,
            busy = "port-forward:${rule.id}" in busyIds,
            onDismiss = { editingPortForward = null },
            onCreate = { },
            onEdit = { id, req ->
                vm.updatePortForward(id, req)
                editingPortForward = null
            }
        )
    }
    deletingPortForward?.let { rule ->
        AlertDialog(
            onDismissRequest = { deletingPortForward = null },
            containerColor = MaterialTheme.colorScheme.surface,
            shape = RoundedCornerShape(16.dp),
            title = { Text("删除端口转发规则", fontSize = 16.sp, fontWeight = FontWeight.Bold) },
            text = { Text("确定要删除\u201C${rule.hostname}:${rule.local_port} → ${rule.target_port}\u201D吗？此操作不可撤销。", fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurfaceVariant) },
            confirmButton = {
                Button(
                    onClick = {
                        vm.deletePortForward(rule.id)
                        deletingPortForward = null
                    },
                    enabled = "port-forward:${rule.id}" !in busyIds,
                    colors = ButtonDefaults.buttonColors(containerColor = OpsRed),
                    shape = RoundedCornerShape(10.dp)
                ) { Text("删除", fontSize = 13.sp) }
            },
            dismissButton = { TextButton(onClick = { deletingPortForward = null }) { Text("取消", fontSize = 13.sp) } }
        )
    }
    if (showCreateHTTPProxy) {
        HTTPProxyFormDialog(
            proxy = null,
            hosts = hosts,
            busy = "http-proxy:create" in busyIds,
            onDismiss = { showCreateHTTPProxy = false },
            onCreate = { req ->
                vm.createHTTPProxy(req)
                showCreateHTTPProxy = false
            },
            onEdit = { _, _ -> }
        )
    }
    editingHTTPProxy?.let { proxy ->
        HTTPProxyFormDialog(
            proxy = proxy,
            hosts = hosts,
            busy = "http-proxy:${proxy.id}" in busyIds,
            onDismiss = { editingHTTPProxy = null },
            onCreate = { },
            onEdit = { id, req ->
                vm.updateHTTPProxy(id, req)
                editingHTTPProxy = null
            }
        )
    }
    deletingHTTPProxy?.let { proxy ->
        AlertDialog(
            onDismissRequest = { deletingHTTPProxy = null },
            containerColor = MaterialTheme.colorScheme.surface,
            shape = RoundedCornerShape(16.dp),
            title = { Text("删除 HTTP 代理", fontSize = 16.sp, fontWeight = FontWeight.Bold) },
            text = { Text("确定要删除\u201C${proxy.name.ifBlank { proxy.hostname }}:${proxy.target_port}\u201D吗？此操作不可撤销。", fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurfaceVariant) },
            confirmButton = {
                Button(
                    onClick = {
                        vm.deleteHTTPProxy(proxy.id)
                        deletingHTTPProxy = null
                    },
                    enabled = "http-proxy:${proxy.id}" !in busyIds,
                    colors = ButtonDefaults.buttonColors(containerColor = OpsRed),
                    shape = RoundedCornerShape(10.dp)
                ) { Text("删除", fontSize = 13.sp) }
            },
            dismissButton = { TextButton(onClick = { deletingHTTPProxy = null }) { Text("取消", fontSize = 13.sp) } }
        )
    }
}

@Composable
private fun OpsOverviewRow(
    overview: SreOverview?,
    onIncidentsClick: () -> Unit,
    onRemediationClick: () -> Unit,
    onTicketsClick: () -> Unit,
    onSloClick: () -> Unit
) {
    val values = listOf(
        OverviewItem("开放事件", overview?.open_incidents ?: 0, OpsRed, onIncidentsClick),
        OverviewItem("待审批", overview?.pending_remediations ?: 0, OpsOrange, onRemediationClick),
        OverviewItem("开放工单", overview?.open_tickets ?: 0, OpsBlue, onTicketsClick),
        OverviewItem("SLO 燃烧", overview?.slo_breaching ?: 0, Color(0xFF8B5CF6), onSloClick)
    )
    // 紧凑单卡横条：4 个关键计数并排、可点击跳转，替代原来占屏的 2×2 大卡片。
    Card(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 10.dp),
        shape = RoundedCornerShape(14.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 0.dp)
    ) {
        Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min).padding(vertical = 12.dp)) {
            values.forEachIndexed { i, item ->
                if (i > 0) {
                    Box(Modifier.width(0.5.dp).fillMaxHeight().background(MaterialTheme.colorScheme.outlineVariant))
                }
                Column(
                    Modifier.weight(1f).clickable(onClick = item.onClick).padding(vertical = 2.dp),
                    horizontalAlignment = Alignment.CenterHorizontally
                ) {
                    Text(item.value.toString(), color = item.color, fontWeight = FontWeight.Black, fontSize = 22.sp)
                    Text(item.label, color = MaterialTheme.colorScheme.onSurfaceVariant, style = MaterialTheme.typography.labelSmall, maxLines = 1)
                }
            }
        }
    }
}

private data class OverviewItem(
    val label: String,
    val value: Int,
    val color: Color,
    val onClick: () -> Unit
)

@Composable
private fun IncidentList(
    incidents: List<Incident>,
    loading: Boolean,
    busyIds: Set<String>,
    onSelect: (Incident) -> Unit,
    onAck: (Long) -> Unit,
    onResolve: (Long) -> Unit,
    onDiagnose: (Incident) -> Unit
) {
    var filter by remember { mutableStateOf("active") }
    val filtered = remember(incidents, filter) {
        incidents.filter {
            when (filter) {
                "active" -> it.status != "resolved"
                "resolved" -> it.status == "resolved"
                else -> true
            }
        }.sortedWith(compareBy<Incident> { if (it.status == "resolved") 1 else 0 }.thenByDescending { it.created_at })
    }
    Column(Modifier.fillMaxSize()) {
        Row(
            Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = 14.dp, vertical = 8.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            listOf("active" to "处理中", "resolved" to "已解决", "all" to "全部").forEach { (key, label) ->
                FilterChip(selected = filter == key, onClick = { filter = key }, label = { Text(label) })
            }
        }
        when {
            loading && incidents.isEmpty() -> LoadingBox()
            filtered.isEmpty() -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Text("当前没有${if (filter == "resolved") "已解决" else "处理中"}事件", color = OpsGreen)
            }
            else -> LazyColumn(
                modifier = Modifier.fillMaxSize().padding(horizontal = 14.dp),
                contentPadding = PaddingValues(bottom = 96.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                items(filtered, key = { it.id }) { incident ->
                    IncidentCard(
                        incident,
                        "incident:${incident.id}" in busyIds,
                        { onSelect(incident) },
                        { onAck(incident.id) },
                        { onResolve(incident.id) },
                        { onDiagnose(incident) },
                        Modifier.animateItem()
                    )
                }
            }
        }
    }
}

@Composable
private fun IncidentCard(
    incident: Incident,
    busy: Boolean,
    onSelect: () -> Unit,
    onAck: () -> Unit,
    onResolve: () -> Unit,
    onDiagnose: () -> Unit,
    modifier: Modifier = Modifier
) {
    val color = severityColor(incident.severity)
    Card(
        onClick = onSelect,
        modifier = modifier,
        shape = RoundedCornerShape(12.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 2.dp)
    ) {
        Row(Modifier.fillMaxWidth().heightIn(min = 120.dp)) {
            Box(Modifier.width(4.dp).fillMaxHeight().background(color))
            Column(Modifier.weight(1f).padding(14.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    StatusPill("#${incident.id}", OpsBlue)
                    Spacer(Modifier.width(6.dp))
                    StatusPill(localizeIncidentStatus(incident.status), incidentStatusColor(incident.status))
                    Spacer(Modifier.weight(1f))
                    Text(formatOpsTime(incident.created_at), color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
                }
                Text(incident.title, color = MaterialTheme.colorScheme.onSurface, fontWeight = FontWeight.Bold, maxLines = 2, overflow = TextOverflow.Ellipsis)
                Text(
                    listOfNotNull(incident.hostname, incident.source, incident.type).joinToString(" · ").ifBlank { "平台事件" },
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    fontSize = 10.sp
                )
                if (incident.status != "resolved") {
                    Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                        if (incident.status == "open") TextButton(onClick = onAck, enabled = !busy) { Text("确认") }
                        TextButton(onClick = onResolve, enabled = !busy) { Text("解决") }
                        TextButton(onClick = onDiagnose, enabled = !busy) {
                            Icon(Icons.Default.AutoAwesome, null, Modifier.size(14.dp))
                            Spacer(Modifier.width(3.dp))
                            Text("AI 诊断")
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun LogsPanel(
    response: LogSearchResponse,
    logsLoading: Boolean,
    busyIds: Set<String>,
    terminalAuditLogs: List<ActivityLogEntry>,
    terminalSessionsLoading: Boolean,
    onSearch: (String, String, Int, Int) -> Unit,
    onDiagnose: (StoredLog) -> Unit,
    onRefreshTerminalSessions: () -> Unit
) {
    var query by remember { mutableStateOf("") }
    var level by remember { mutableStateOf("") }
    var since by remember { mutableIntStateOf(360) }
    var logSourceTab by remember { mutableIntStateOf(0) } // 0=Agent日志, 1=终端审计

    // 切换到终端审计 Tab 时自动加载数据
    LaunchedEffect(logSourceTab) {
        if (logSourceTab == 1) onRefreshTerminalSessions()
    }

    Column(Modifier.fillMaxSize()) {
        // 日志源切换 Tab
        TabRow(selectedTabIndex = logSourceTab, Modifier.fillMaxWidth()) {
            Tab(
                selected = logSourceTab == 0,
                onClick = { logSourceTab = 0 },
                text = { Text("日志检索", fontSize = 12.sp) },
                icon = { Icon(Icons.Default.Search, null, Modifier.size(16.dp)) }
            )
            Tab(
                selected = logSourceTab == 1,
                onClick = { logSourceTab = 1 },
                text = { Text("终端审计", fontSize = 12.sp) },
                icon = { Icon(Icons.Default.Terminal, null, Modifier.size(16.dp)) }
            )
        }

        when (logSourceTab) {
            0 -> {
                // ── Agent 日志面板 ──
                OutlinedTextField(
                    value = query,
                    onValueChange = { query = it },
                    modifier = Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 8.dp),
                    placeholder = { Text("搜索日志关键字") },
                    leadingIcon = { Icon(Icons.Default.Search, null) },
                    trailingIcon = {
                        IconButton(onClick = { onSearch(query, level, since, 1) }) {
                            Icon(Icons.AutoMirrored.Filled.Send, "检索")
                        }
                    },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
                    keyboardActions = KeyboardActions(onSearch = { onSearch(query, level, since, 1) })
                )
                Row(
                    Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = 14.dp),
                    horizontalArrangement = Arrangement.spacedBy(7.dp)
                ) {
                    listOf("" to "全部", "error" to "ERROR", "warn" to "WARN", "info" to "INFO").forEach { (key, label) ->
                        FilterChip(
                            selected = level == key,
                            onClick = { level = key; onSearch(query, key, since, 1) },
                            label = { Text(label, fontSize = 10.sp) }
                        )
                    }
                    listOf(60 to "1h", 360 to "6h", 1440 to "24h").forEach { (minutes, label) ->
                        FilterChip(
                            selected = since == minutes,
                            onClick = { since = minutes; onSearch(query, level, minutes, 1) },
                            label = { Text(label, fontSize = 10.sp) }
                        )
                    }
                }
                LogStatsRow(response)
                if (logsLoading) {
                    Box(Modifier.fillMaxWidth().height(3.dp).background(OpsBlue.copy(alpha = 0.25f)))
                }
                if (response.items.isEmpty() && !logsLoading) {
                    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                        Text("无匹配日志；被控端需配置 --log-paths", color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                } else {
                    LazyColumn(
                        Modifier.weight(1f).padding(horizontal = 14.dp),
                        contentPadding = PaddingValues(bottom = 16.dp),
                        verticalArrangement = Arrangement.spacedBy(7.dp)
                    ) {
                        items(response.items, key = { "${it.ts}:${it.host_id}:${it.message.hashCode()}" }) { log ->
                            LogCard(log, "log:${log.ts}:${log.host_id}" in busyIds) { onDiagnose(log) }
                        }
                        if (response.pages > 1) {
                            item {
                                Row(Modifier.fillMaxWidth(), Arrangement.Center, Alignment.CenterVertically) {
                                    TextButton(
                                        onClick = { onSearch(query, level, since, response.page - 1) },
                                        enabled = response.page > 1 && !logsLoading
                                    ) { Text("上一页") }
                                    Text("${response.page} / ${response.pages} · 共 ${response.total} 条", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
                                    TextButton(
                                        onClick = { onSearch(query, level, since, response.page + 1) },
                                        enabled = response.page < response.pages && !logsLoading
                                    ) { Text("下一页") }
                                }
                            }
                        }
                    }
                }
            }

            1 -> {
                // ── 终端审计面板 ──
                var terminalSearchQuery by remember { mutableStateOf("") }
                OutlinedTextField(
                    value = terminalSearchQuery,
                    onValueChange = { terminalSearchQuery = it },
                    modifier = Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 8.dp),
                    placeholder = { Text("搜索命令、主机名、操作者…") },
                    leadingIcon = { Icon(Icons.Default.Search, null) },
                    trailingIcon = {
                        if (terminalSearchQuery.isNotEmpty()) {
                            IconButton(onClick = { terminalSearchQuery = "" }) {
                                Icon(Icons.Default.Close, "清除搜索")
                            }
                        }
                    },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search)
                )
                val filteredAuditLogs = remember(terminalAuditLogs, terminalSearchQuery) {
                    if (terminalSearchQuery.isBlank()) terminalAuditLogs
                    else {
                        val q = terminalSearchQuery.lowercase()
                        terminalAuditLogs.filter { entry ->
                            entry.message.lowercase().contains(q) ||
                            entry.host.lowercase().contains(q) ||
                            entry.actor.lowercase().contains(q)
                        }
                    }
                }
                Row(
                    Modifier.fillMaxWidth().padding(horizontal = 14.dp),
                    horizontalArrangement = Arrangement.SpaceBetween,
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Text(
                        "终端命令审计（共 ${filteredAuditLogs.size}/${terminalAuditLogs.size} 条）",
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        fontSize = 12.sp
                    )
                    IconButton(
                        onClick = onRefreshTerminalSessions,
                        enabled = !terminalSessionsLoading
                    ) {
                        Icon(Icons.Default.Refresh, "刷新", tint = OpsBlue)
                    }
                }
                when {
                    terminalSessionsLoading && terminalAuditLogs.isEmpty() -> LoadingBox()
                    filteredAuditLogs.isEmpty() -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                        Text(
                            if (terminalSearchQuery.isNotBlank()) "无匹配的审计记录" else "暂无终端命令审计记录",
                            color = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    }
                    else -> LazyColumn(
                        Modifier.weight(1f).padding(horizontal = 14.dp),
                        contentPadding = PaddingValues(bottom = 16.dp),
                        verticalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        items(filteredAuditLogs, key = { "${it.timestamp}:${it.actor}:${it.message.hashCode()}" }) { entry ->
                            TerminalCommandAuditCard(entry)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun TerminalCommandAuditCard(entry: ActivityLogEntry) {
    // 解析消息："终端命令 [hostname hostIP]: cmd"
    val cmdText = remember(entry.message) {
        val idx = entry.message.indexOf("]: ")
        if (idx >= 0) entry.message.substring(idx + 3) else entry.message
    }
    val hostIpText = remember(entry.message) {
        val start = entry.message.indexOf('[')
        val end = entry.message.indexOf(']')
        if (start >= 0 && end > start) entry.message.substring(start + 1, end) else ""
    }
    val levelColor = when (entry.level) {
        "critical", "error" -> OpsRed
        "warning", "warn" -> OpsOrange
        else -> OpsGreen
    }
    Card(
        shape = RoundedCornerShape(12.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 2.dp)
    ) {
        Column(Modifier.fillMaxWidth().padding(14.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            // 顶部：图标 + 主机名 + 时间
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Default.Terminal, null, Modifier.size(16.dp), tint = levelColor)
                Spacer(Modifier.width(6.dp))
                Text(
                    entry.host.ifBlank { hostIpText.substringBefore(' ') },
                    fontWeight = FontWeight.Bold,
                    color = MaterialTheme.colorScheme.onSurface,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f)
                )
                Text(
                    formatOpsTime(entry.timestamp),
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    fontSize = 10.sp
                )
            }
            // 命令内容（重点突出，可选中复制）
            SelectionContainer {
                Text(
                    cmdText,
                    color = MaterialTheme.colorScheme.onSurface,
                    fontSize = 12.sp,
                    fontFamily = androidx.compose.ui.text.font.FontFamily.Monospace,
                    modifier = Modifier
                        .fillMaxWidth()
                        .background(
                            MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.5f),
                            RoundedCornerShape(6.dp)
                        )
                        .padding(horizontal = 10.dp, vertical = 8.dp)
                )
            }
            // 底部：操作者 + IP
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Text(
                    "操作者: ${entry.actor.ifBlank { "—" }}",
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    fontSize = 11.sp
                )
                if (hostIpText.isNotBlank()) {
                    val ip = hostIpText.substringAfter(' ', "")
                    if (ip.isNotBlank()) {
                        Text("IP: $ip", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 11.sp)
                    }
                }
            }
        }
    }
}

@Composable
private fun LogStatsRow(response: LogSearchResponse) {
    val stats = response.stats.by_level
    Row(
        Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = 14.dp, vertical = 7.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        listOf(
            "匹配" to response.total,
            "错误" to (stats["error"] ?: 0),
            "警告" to (stats["warn"] ?: 0),
            "信息" to (stats["info"] ?: 0)
        ).forEach { (label, value) ->
            Text("$label $value", color = if (label == "错误") OpsRed else MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
        }
    }
}

@Composable
private fun LogCard(log: StoredLog, diagnosing: Boolean, onDiagnose: () -> Unit) {
    val color = logLevelColor(log.level)
    Card(
        shape = RoundedCornerShape(10.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceVariant),
        elevation = CardDefaults.cardElevation(defaultElevation = 1.dp)
    ) {
        Column(Modifier.fillMaxWidth().padding(10.dp), verticalArrangement = Arrangement.spacedBy(5.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                StatusPill(log.level.uppercase(), color)
                Spacer(Modifier.width(7.dp))
                Text(log.hostname.ifBlank { log.host_id }, color = OpsBlue, fontSize = 10.sp, modifier = Modifier.weight(1f), maxLines = 1)
                Text(formatOpsTime(log.ts), color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
                if (log.level == "error" || log.level == "warn") {
                    IconButton(onClick = onDiagnose, enabled = !diagnosing, modifier = Modifier.size(30.dp)) {
                        if (diagnosing) CircularProgressIndicator(Modifier.size(14.dp), strokeWidth = 2.dp)
                        else Icon(Icons.Default.BugReport, "诊断日志", tint = color, modifier = Modifier.size(17.dp))
                    }
                }
            }
            SelectionContainer {
                Text(log.message, color = MaterialTheme.colorScheme.onSurface, fontFamily = FontFamily.Monospace, fontSize = 11.sp, lineHeight = 16.sp)
            }
            if (log.source.isNotBlank()) Text(log.source, color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
        }
    }
}

@Composable
private fun PlaybooksPanel(
    playbooks: List<Playbook>,
    executions: List<PlaybookExecution>,
    busyIds: Set<String>,
    onExecute: (Playbook) -> Unit,
    onExecutionClick: (PlaybookExecution) -> Unit,
    onRefreshExecutions: () -> Unit
) {
    LazyColumn(
        Modifier.fillMaxSize().padding(horizontal = 14.dp),
        contentPadding = PaddingValues(top = 10.dp, bottom = 20.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        item {
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text("剧本", color = MaterialTheme.colorScheme.onSurface, fontWeight = FontWeight.Bold)
                    Text("选择剧本执行远程命令", color = MaterialTheme.colorScheme.onSurfaceVariant, style = MaterialTheme.typography.labelSmall)
                }
            }
        }
        if (playbooks.isEmpty()) {
            item {
                Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface)) {
                    Text(
                        "暂无自动化剧本，请先在 Web 端创建并审核步骤。已有执行历史仍可在下方查看。",
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        style = MaterialTheme.typography.bodySmall,
                        modifier = Modifier.fillMaxWidth().padding(16.dp)
                    )
                }
            }
        } else {
            items(playbooks, key = { it.id }) { playbook ->
                PlaybookCard(playbook, "playbook:${playbook.id}" in busyIds) { onExecute(playbook) }
            }
        }

        item {
            HorizontalDivider(Modifier.padding(top = 5.dp), color = MaterialTheme.colorScheme.outlineVariant)
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text("执行历史", color = MaterialTheme.colorScheme.onSurface, fontWeight = FontWeight.Bold)
                }
                IconButton(onClick = onRefreshExecutions) {
                    Icon(Icons.Default.Refresh, contentDescription = "刷新执行记录", tint = OpsBlue)
                }
            }
        }
        if (executions.isEmpty()) {
            item { Text("暂无执行记录", color = MaterialTheme.colorScheme.onSurfaceVariant, style = MaterialTheme.typography.bodySmall) }
        } else {
            items(executions.sortedByDescending { it.start_time }.take(10), key = { it.id }) { execution ->
                ExecutionCard(execution) { onExecutionClick(execution) }
            }
        }
    }
}

@Composable
private fun GovernancePanel(
    slos: List<SloStatus>,
    tickets: List<Ticket>,
    remediationRuns: List<RemediationRun>,
    busyIds: Set<String>,
    govTab: Int,
    onGovTabChange: (Int) -> Unit,
    onDecideRemediation: (Long, Boolean) -> Unit,
    onResolveTicket: (Ticket) -> Unit,
    onSelectTicket: (Ticket) -> Unit
) {
    val pending = remediationRuns.filter { it.status == "pending_approval" }
    val openTickets = tickets.filter { it.status != "resolved" && it.status != "closed" }

    Column(Modifier.fillMaxSize()) {
    TabRow(
        selectedTabIndex = govTab,
        containerColor = MaterialTheme.colorScheme.surfaceVariant,
        contentColor = OpsBlue,
        divider = {}
    ) {
        listOf(
            "审批" to "${if (pending.isNotEmpty()) " ${pending.size}" else ""}",
            "SLO" to "${if (slos.isNotEmpty()) " ${slos.size}" else ""}",
            "工单" to "${if (openTickets.isNotEmpty()) " ${openTickets.size}" else ""}"
        ).forEachIndexed { index, (title, badge) ->
            Tab(
                selected = govTab == index,
                onClick = { onGovTabChange(index) },
                text = { Text("$title$badge", fontWeight = FontWeight.Bold) }
            )
        }
    }

    Box(Modifier.weight(1f).fillMaxWidth()) {
    when (govTab) {
        0 -> {
            // ── 审批 Tab ──
            if (pending.isEmpty()) {
                Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    Text("暂无待审批项", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 12.sp)
                }
            } else {
                LazyColumn(
                    Modifier.fillMaxSize().padding(horizontal = 14.dp),
                    contentPadding = PaddingValues(top = 10.dp, bottom = 22.dp),
                    verticalArrangement = Arrangement.spacedBy(9.dp)
                ) {
                    items(pending, key = { "remediation:${it.id}" }) { run ->
                        RemediationCard(
                            run,
                            "remediation:${run.id}" in busyIds,
                            { onDecideRemediation(run.id, true) },
                            { onDecideRemediation(run.id, false) },
                            Modifier.animateItem()
                        )
                    }
                }
            }
        }
        1 -> {
            // ── SLO Tab ──
            if (slos.isEmpty()) {
                Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    Text("暂无 SLO 配置", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 12.sp)
                }
            } else {
                LazyColumn(
                    Modifier.fillMaxSize().padding(horizontal = 14.dp),
                    contentPadding = PaddingValues(top = 10.dp, bottom = 22.dp),
                    verticalArrangement = Arrangement.spacedBy(9.dp)
                ) {
                    items(slos, key = { "slo:${it.id}" }) { slo -> SloCard(slo, Modifier.animateItem()) }
                }
            }
        }
        else -> {
            // ── 工单 Tab ──
            if (openTickets.isEmpty()) {
                Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    Text("暂无开放工单", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 12.sp)
                }
            } else {
                LazyColumn(
                    Modifier.fillMaxSize().padding(horizontal = 14.dp),
                    contentPadding = PaddingValues(top = 10.dp, bottom = 22.dp),
                    verticalArrangement = Arrangement.spacedBy(9.dp)
                ) {
                    items(openTickets, key = { "ticket:${it.id}" }) { ticket ->
                        TicketCard(ticket, "ticket:${ticket.id}" in busyIds, { onSelectTicket(ticket) }, { onResolveTicket(ticket) }, Modifier.animateItem())
                    }
                }
            }
        }
    }
    }
    }
}

@Composable
private fun RemediationCard(run: RemediationRun, busy: Boolean, onApprove: () -> Unit, onReject: () -> Unit, modifier: Modifier = Modifier) {
    Card(
        modifier = modifier,
        shape = RoundedCornerShape(12.dp),
        colors = CardDefaults.cardColors(containerColor = OpsOrange.copy(alpha = 0.08f)),
        elevation = CardDefaults.cardElevation(defaultElevation = 2.dp)
    ) {
        Column(Modifier.fillMaxWidth().padding(12.dp), verticalArrangement = Arrangement.spacedBy(7.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                StatusPill("待审批", OpsOrange)
                Spacer(Modifier.width(7.dp))
                Text(run.rule_name, color = MaterialTheme.colorScheme.onSurface, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f), maxLines = 1)
                Text(formatOpsTime(run.created_at), color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
            }
            Text("${run.hostname} · ${run.alert_type} → ${run.playbook_name}", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 11.sp)
            run.reason?.takeIf { it.isNotBlank() }?.let { Text(it, color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp) }
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                TextButton(onClick = onReject, enabled = !busy) { Text("拒绝", color = OpsRed) }
                Button(onClick = onApprove, enabled = !busy, colors = ButtonDefaults.buttonColors(containerColor = OpsOrange)) {
                    if (busy) CircularProgressIndicator(Modifier.size(14.dp), strokeWidth = 2.dp, color = MaterialTheme.colorScheme.onSurface)
                    else Text("批准")
                }
            }
        }
    }
}

@Composable
private fun SloCard(slo: SloStatus, modifier: Modifier = Modifier) {
    val color = if (slo.breaching) OpsRed else OpsGreen
    Card(
        modifier = modifier,
        shape = RoundedCornerShape(12.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 2.dp)
    ) {
        Column(Modifier.fillMaxWidth().padding(14.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                StatusDot(color, 8.dp)
                Spacer(Modifier.width(7.dp))
                Text(slo.name, color = MaterialTheme.colorScheme.onSurface, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
                StatusPill(if (slo.breaching) "超预算" else "正常", color)
            }
            Text("SLI %.3f%%  目标 %.3f%%  %d天窗口".format(slo.sli, slo.target, slo.window_days), color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
            Box(Modifier.fillMaxWidth().height(6.dp).background(MaterialTheme.colorScheme.outlineVariant, RoundedCornerShape(3.dp))) {
                Box(
                    Modifier.fillMaxWidth((slo.error_budget / 100.0).toFloat().coerceIn(0f, 1f))
                        .fillMaxHeight().background(color, RoundedCornerShape(3.dp))
                )
            }
            Text("预算 %.1f%% · 燃烧率 %.2fx · %,d/%,d".format(slo.error_budget, slo.burn_rate, slo.good_events, slo.total_events), color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
        }
    }
}

@Composable
private fun TicketCard(ticket: Ticket, busy: Boolean, onClick: () -> Unit, onResolve: () -> Unit, modifier: Modifier = Modifier) {
    val priority = (ticket.priority ?: "p3").ifBlank { "p3" }
    val title = (ticket.title ?: "").ifBlank { "（无标题）" }
    val assignee = ticket.assignee ?: ""
    val color = when (priority.lowercase()) { "p1" -> OpsRed; "p2" -> OpsOrange; else -> OpsBlue }
    Card(
        onClick = onClick,
        modifier = modifier,
        shape = RoundedCornerShape(12.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 2.dp)
    ) {
        Column(Modifier.fillMaxWidth().padding(14.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            // 标题行：优先级标签 + 编号 + 标题（顶对齐避免多行时标签偏移）
            Row(verticalAlignment = Alignment.Top) {
                StatusPill(priority.uppercase(), color)
                Spacer(Modifier.width(7.dp))
                Text("#${ticket.id}", color = OpsBlue, fontWeight = FontWeight.Bold, fontSize = 12.sp)
                Spacer(Modifier.width(6.dp))
                Text(
                    title,
                    color = MaterialTheme.colorScheme.onSurface,
                    fontWeight = FontWeight.Bold,
                    fontSize = 12.sp,
                    modifier = Modifier.weight(1f),
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis
                )
            }
            // 元信息行：负责人、关联事件分行展示
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Text(
                    if (assignee.isNotBlank()) "负责人 $assignee" else "待分配",
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    fontSize = 10.sp,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f)
                )
                if (ticket.incident_id > 0) {
                    Text("关联事件 #${ticket.incident_id}", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp, maxLines = 1)
                }
            }
            // 操作行
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                OutlinedButton(onClick = onResolve, enabled = !busy) {
                    if (busy) CircularProgressIndicator(Modifier.size(14.dp), strokeWidth = 2.dp)
                    else Text("解决")
                }
            }
        }
    }
}

@Composable
private fun PlaybookCard(playbook: Playbook, busy: Boolean, onExecute: () -> Unit) {
    val steps = playbook.steps.orEmpty()
    Card(
        shape = RoundedCornerShape(12.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 2.dp)
    ) {
        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Default.Terminal, null, tint = OpsBlue, modifier = Modifier.size(20.dp))
                Spacer(Modifier.width(8.dp))
                Column(Modifier.weight(1f)) {
                    Text(playbook.name, color = MaterialTheme.colorScheme.onSurface, fontWeight = FontWeight.Bold)
                    Text("${steps.size} 步 · ${scheduleText(playbook)}", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
                }
                Button(
                    onClick = onExecute,
                    enabled = !busy,
                    contentPadding = PaddingValues(horizontal = 12.dp),
                    colors = ButtonDefaults.buttonColors(containerColor = OpsBlue)
                ) {
                    if (busy) CircularProgressIndicator(Modifier.size(15.dp), strokeWidth = 2.dp, color = MaterialTheme.colorScheme.onSurface)
                    else Icon(Icons.Default.PlayArrow, null, modifier = Modifier.size(17.dp))
                    Spacer(Modifier.width(3.dp))
                    Text(if (busy) "执行中" else "执行")
                }
            }
            playbook.description?.takeIf { it.isNotBlank() }?.let { Text(it, color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 11.sp) }
            steps.take(3).forEachIndexed { index, step ->
                Text(
                    "${index + 1}. ${step.name} → ${step.target}",
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    fontFamily = FontFamily.Monospace,
                    fontSize = 10.sp,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
            }
            if (steps.size > 3) Text("另有 ${steps.size - 3} 个步骤…", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
        }
    }
}

@Composable
private fun ExecutionCard(execution: PlaybookExecution, onClick: () -> Unit) {
    val color = executionStatusColor(execution.status)
    val hr = execution.host_results ?: emptyMap()
    val success = hr.values.count { it.status == "success" }
    val failed = hr.values.count { it.status in setOf("failed", "timeout") }
    Card(
        onClick = onClick,
        shape = RoundedCornerShape(12.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceVariant),
        elevation = CardDefaults.cardElevation(defaultElevation = 2.dp)
    ) {
        Row(Modifier.fillMaxWidth().padding(11.dp), verticalAlignment = Alignment.CenterVertically) {
            StatusDot(color, 8.dp)
            Spacer(Modifier.width(8.dp))
            Column(Modifier.weight(1f)) {
                Text(execution.playbook_name, color = MaterialTheme.colorScheme.onSurface, fontSize = 12.sp, fontWeight = FontWeight.Bold)
                Text(
                    "#${execution.id} · ${hr.size} 台 · 成功 $success${if (failed > 0) " · 失败 $failed" else ""} · ${formatOpsTime(execution.start_time)}",
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    style = MaterialTheme.typography.labelSmall,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
            }
            StatusPill(localizeExecutionStatus(execution.status), color)
            Icon(Icons.Default.ChevronRight, contentDescription = "查看执行详情", tint = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

@Composable
private fun CreateIncidentDialog(busy: Boolean, onDismiss: () -> Unit, onCreate: (String, String) -> Unit) {
    var title by remember { mutableStateOf("") }
    var severity by remember { mutableStateOf("warning") }
    AlertDialog(
        onDismissRequest = { if (!busy) onDismiss() },
        containerColor = MaterialTheme.colorScheme.surface,
        shape = RoundedCornerShape(16.dp),
        title = { Text("新建运维事件", fontSize = 16.sp, fontWeight = FontWeight.Bold) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                OutlinedTextField(title, { title = it }, modifier = Modifier.fillMaxWidth(), label = { Text("事件标题") }, singleLine = true)
                Row(horizontalArrangement = Arrangement.spacedBy(7.dp)) {
                    listOf("critical" to "严重", "warning" to "警告", "info" to "信息").forEach { (key, label) ->
                        FilterChip(selected = severity == key, onClick = { severity = key }, label = { Text(label) })
                    }
                }
            }
        },
        confirmButton = {
            Button(onClick = { onCreate(title, severity) }, enabled = title.isNotBlank() && !busy, shape = RoundedCornerShape(10.dp)) {
                if (busy) CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp, color = MaterialTheme.colorScheme.onSurface)
                else Text("创建", fontSize = 13.sp)
            }
        },
        dismissButton = { TextButton(onClick = onDismiss, enabled = !busy) { Text("取消", fontSize = 13.sp) } }
    )
}

@Composable
private fun IncidentDetailDialog(
    incident: Incident,
    busy: Boolean,
    onDismiss: () -> Unit,
    onAck: () -> Unit,
    onResolve: () -> Unit,
    onEscalate: () -> Unit,
    onDiagnose: () -> Unit
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        containerColor = MaterialTheme.colorScheme.surface,
        shape = RoundedCornerShape(16.dp),
        title = {
            Column {
                Text("#${incident.id} ${incident.title}", maxLines = 2, overflow = TextOverflow.Ellipsis, fontSize = 16.sp, fontWeight = FontWeight.Bold)
                Text("${localizeIncidentStatus(incident.status)} · ${incident.hostname ?: incident.source}", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
            }
        },
        text = {
            Column {
                Text("事件时间线", color = OpsBlue, fontWeight = FontWeight.Bold)
                Spacer(Modifier.height(8.dp))
                LazyColumn(Modifier.fillMaxWidth().heightIn(max = 340.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    items(incident.timeline.sortedByDescending { it.ts }) { event ->
                        Row {
                            StatusDot(incidentStatusColor(incident.status), 7.dp)
                            Spacer(Modifier.width(8.dp))
                            Column {
                                Text("${event.kind} · ${formatOpsTime(event.ts)}", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
                                Text(event.text, color = MaterialTheme.colorScheme.onSurface, fontSize = 11.sp)
                                event.actor?.let { Text("操作人：$it", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp) }
                            }
                        }
                    }
                }
            }
        },
        confirmButton = {
            if (incident.status != "resolved") {
                Button(onClick = onResolve, enabled = !busy) { Text("解决") }
            } else TextButton(onClick = onDismiss) { Text("关闭") }
        },
        dismissButton = {
            Row {
                TextButton(onClick = onDiagnose, enabled = !busy) {
                    Icon(Icons.Default.AutoAwesome, null, modifier = Modifier.size(15.dp))
                    Spacer(Modifier.width(4.dp))
                    Text("AI 诊断")
                }
                if (incident.status == "open") TextButton(onClick = onAck, enabled = !busy) { Text("确认") }
                if (incident.ticket_id <= 0) TextButton(onClick = onEscalate, enabled = !busy) { Text("转工单") }
            }
        }
    )
}

@Composable
private fun TicketDetailDialog(
    ticket: Ticket,
    busy: Boolean,
    onDismiss: () -> Unit,
    onResolve: () -> Unit,
    onAssign: (String) -> Unit,
    onComment: (String, List<Map<String, Any?>>) -> Unit,
    loadUsers: suspend () -> List<com.aiops.monitor.data.models.DirectoryUser>
) {
    val priority = (ticket.priority ?: "p3").ifBlank { "p3" }
    val status = (ticket.status ?: "open").ifBlank { "open" }
    val color = when (priority.lowercase()) { "p1" -> OpsRed; "p2" -> OpsOrange; else -> OpsBlue }
    var users by remember { mutableStateOf<List<com.aiops.monitor.data.models.DirectoryUser>>(emptyList()) }
    var assignee by remember(ticket.id, ticket.assignee) { mutableStateOf(ticket.assignee.orEmpty()) }
    var commentText by remember(ticket.id) { mutableStateOf("") }
    var assigneeMenu by remember { mutableStateOf(false) }
    LaunchedEffect(ticket.id) {
        users = loadUsers()
        if (assignee.isNotBlank() && users.none { it.username == assignee }) {
            users = users + com.aiops.monitor.data.models.DirectoryUser(username = assignee, label = "$assignee（历史）")
        }
    }
    AlertDialog(
        onDismissRequest = onDismiss,
        containerColor = MaterialTheme.colorScheme.surface,
        shape = RoundedCornerShape(16.dp),
        title = {
            Column {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    StatusPill(priority.uppercase(), color)
                    Spacer(Modifier.width(8.dp))
                    Text("#${ticket.id}", color = OpsBlue, fontWeight = FontWeight.Bold, fontSize = 16.sp)
                }
                Spacer(Modifier.height(6.dp))
                Text((ticket.title ?: "").ifBlank { "（无标题）" }, maxLines = 3, overflow = TextOverflow.Ellipsis, fontSize = 14.sp)
            }
        },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Row(
                    Modifier.fillMaxWidth().background(MaterialTheme.colorScheme.surfaceVariant, RoundedCornerShape(8.dp)).padding(10.dp),
                    horizontalArrangement = Arrangement.spacedBy(16.dp)
                ) {
                    Column(Modifier.weight(1f)) {
                        Text("状态", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
                        Text(localizeTicketStatus(status), fontWeight = FontWeight.Bold, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    }
                    Column(Modifier.weight(1f)) {
                        Text("负责人", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
                        Box {
                            TextButton(onClick = { assigneeMenu = true }, contentPadding = PaddingValues(0.dp)) {
                                Text(assignee.ifBlank { "未指派 ▾" }, fontWeight = FontWeight.Bold, maxLines = 1, overflow = TextOverflow.Ellipsis, fontSize = 13.sp)
                            }
                            DropdownMenu(expanded = assigneeMenu, onDismissRequest = { assigneeMenu = false }) {
                                DropdownMenuItem(text = { Text("未指派") }, onClick = {
                                    assignee = ""; assigneeMenu = false; onAssign("")
                                })
                                users.forEach { u ->
                                    DropdownMenuItem(text = { Text(u.label.ifBlank { u.username }) }, onClick = {
                                        assignee = u.username; assigneeMenu = false; onAssign(u.username)
                                    })
                                }
                            }
                        }
                    }
                }
                if (!(ticket.description ?: "").isBlank()) {
                    Text("描述", color = OpsBlue, fontWeight = FontWeight.Bold, fontSize = 11.sp)
                    Text(
                        ticket.description ?: "",
                        color = MaterialTheme.colorScheme.onSurface,
                        fontSize = 12.sp,
                        overflow = TextOverflow.Ellipsis
                    )
                }
                if (ticket.incident_id > 0) {
                    Text("关联事件 #${ticket.incident_id}", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 11.sp)
                }
                val comments = ticket.comments.orEmpty()
                if (comments.isNotEmpty()) {
                    Text("评论 (${comments.size})", color = OpsBlue, fontWeight = FontWeight.Bold, fontSize = 11.sp)
                    LazyColumn(Modifier.fillMaxWidth().heightIn(max = 160.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                        items(comments.sortedByDescending { it.ts }) { comment ->
                            Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceVariant)) {
                                Column(Modifier.padding(8.dp)) {
                                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                        Text((comment.author ?: "").ifBlank { "系统" }, fontWeight = FontWeight.Bold, fontSize = 10.sp)
                                        Text(formatOpsTime(comment.ts), color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
                                    }
                                    Text((comment.text ?: "").ifBlank { "（空评论）" }, fontSize = 11.sp)
                                    val atts = comment.attachments.orEmpty()
                                    if (atts.isNotEmpty()) {
                                        Text(atts.joinToString { "📎 ${it.name.ifBlank { it.kind }}" }, fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                    }
                                }
                            }
                        }
                    }
                }
                OutlinedTextField(
                    value = commentText,
                    onValueChange = { commentText = it },
                    modifier = Modifier.fillMaxWidth(),
                    placeholder = { Text("添加评论…", fontSize = 12.sp) },
                    maxLines = 3,
                    shape = RoundedCornerShape(10.dp)
                )
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Button(
                        onClick = {
                            if (commentText.isNotBlank()) {
                                onComment(commentText.trim(), emptyList())
                                commentText = ""
                            }
                        },
                        enabled = !busy && commentText.isNotBlank(),
                        shape = RoundedCornerShape(10.dp)
                    ) { Text("发送评论", fontSize = 12.sp) }
                }
            }
        },
        confirmButton = {
            if (status != "resolved" && status != "closed") {
                Button(onClick = onResolve, enabled = !busy) { Text("解决") }
            } else TextButton(onClick = onDismiss) { Text("关闭") }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text("取消") }
        }
    )
}

private fun localizeTicketStatus(status: String) = when (status) {
    "open" -> "待处理"
    "in_progress" -> "处理中"
    "resolved" -> "已解决"
    "closed" -> "已关闭"
    else -> status
}

@Composable
private fun IncidentDiagnosisDialog(
    state: IncidentDiagnosisUiState,
    onDismiss: () -> Unit,
    onTerminalContextChanged: (Boolean) -> Unit,
    onSend: (String) -> Unit,
    onFeedback: (Boolean) -> Unit
) {
    var input by remember(state.incident.id) { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false),
        modifier = Modifier.fillMaxSize(),
        title = null,
        confirmButton = {},
        text = {
            Column(Modifier.fillMaxSize().padding(bottom = 6.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(Icons.Default.AutoAwesome, null, tint = OpsBlue, modifier = Modifier.size(22.dp))
                    Spacer(Modifier.width(8.dp))
                    Column(Modifier.weight(1f)) {
                        Text("事件 AI 诊断", color = MaterialTheme.colorScheme.onSurface, fontWeight = FontWeight.Black, fontSize = 18.sp)
                        Text("#${state.incident.id} ${state.incident.title}", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    }
                    IconButton(onClick = onDismiss) {
                        Icon(Icons.Default.Close, "关闭诊断", tint = MaterialTheme.colorScheme.onSurface)
                    }
                }

                Card(
                    colors = CardDefaults.cardColors(containerColor = if (state.running) OpsBlue.copy(alpha = 0.10f) else OpsGreen.copy(alpha = 0.08f)),
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Row(Modifier.padding(10.dp), verticalAlignment = Alignment.CenterVertically) {
                        if (state.running) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp, color = OpsBlue)
                        else StatusDot(if (state.error == null) OpsGreen else OpsRed, 8.dp)
                        Spacer(Modifier.width(9.dp))
                        Column {
                            Text(state.activity, color = MaterialTheme.colorScheme.onSurface, fontWeight = FontWeight.SemiBold, fontSize = 12.sp)
                            Text(
                                if (state.running) "窗口保持打开，诊断结果会实时显示在下方" else "可继续追问，回答会保留事件上下文",
                                color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp
                            )
                        }
                    }
                }

                Spacer(Modifier.height(8.dp))
                LazyColumn(
                    Modifier.weight(1f).fillMaxWidth(),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                    contentPadding = PaddingValues(vertical = 4.dp)
                ) {
                    if (state.messages.isEmpty() && state.running) {
                        item {
                            AnimatedDotsText("正在建立诊断上下文，请稍候", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 11.sp)
                        }
                    }
                    itemsIndexed(state.messages) { index, message ->
                        val assistant = message.role != "user"
                        Row(Modifier.fillMaxWidth(), horizontalArrangement = if (assistant) Arrangement.Start else Arrangement.End) {
                            Card(
                                modifier = Modifier.fillMaxWidth(if (assistant) 0.94f else 0.84f),
                                shape = RoundedCornerShape(11.dp),
                                colors = CardDefaults.cardColors(
                                    containerColor = if (assistant) MaterialTheme.colorScheme.surface else OpsBlue.copy(alpha = 0.18f)
                                )
                            ) {
                                Column(Modifier.padding(11.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                                    Text(if (assistant) "AIOps 诊断引擎" else "我", color = if (assistant) OpsBlue else Color(0xFF9CB8FF), fontSize = 10.sp, fontWeight = FontWeight.Bold)
                                    if (message.content.isBlank() && state.running && index == state.messages.lastIndex) {
                                        Row(verticalAlignment = Alignment.CenterVertically) {
                                            CircularProgressIndicator(Modifier.size(14.dp), strokeWidth = 2.dp, color = OpsBlue)
                                            Spacer(Modifier.width(7.dp))
                                            AnimatedDotsText("正在生成回复", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 11.sp)
                                        }
                                    } else {
                                        SelectionContainer {
                                            MarkdownText(
                                                text = message.content,
                                                color = MaterialTheme.colorScheme.onSurface,
                                                fontSize = 12.sp,
                                                lineHeight = 18.sp
                                            )
                                        }
                                    }
                                }
                            }
                        }
                    }
                    state.error?.let { message ->
                        item {
                            Card(colors = CardDefaults.cardColors(containerColor = OpsRed.copy(alpha = 0.10f))) {
                                Column(Modifier.fillMaxWidth().padding(11.dp), verticalArrangement = Arrangement.spacedBy(5.dp)) {
                                    Text("诊断服务异常", color = OpsRed, fontWeight = FontWeight.Bold)
                                    SelectionContainer { Text(message, color = Color(0xFFFFB6B6), fontSize = 11.sp) }
                                    Text("事件详情仍可正常处理；请检查服务端 AI 配置后重新进入诊断。", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
                                }
                            }
                        }
                    }
                }

                HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                    Icon(Icons.Default.Terminal, null, tint = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.size(16.dp))
                    Spacer(Modifier.width(6.dp))
                    Column(Modifier.weight(1f)) {
                        Text("结合终端操作记录", color = MaterialTheme.colorScheme.onSurface, fontSize = 11.sp)
                        Text("仅发送该事件关联主机的终端摘要", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 9.sp)
                    }
                    Switch(checked = state.includeTerminal, onCheckedChange = onTerminalContextChanged, enabled = !state.running)
                }
                OutlinedTextField(
                    value = input,
                    onValueChange = { input = it },
                    modifier = Modifier.fillMaxWidth(),
                    enabled = !state.running,
                    placeholder = { Text("追问根因、验证方法或回滚步骤…") },
                    maxLines = 3,
                    trailingIcon = {
                        IconButton(
                            onClick = { val text = input; input = ""; onSend(text) },
                            enabled = input.isNotBlank() && !state.running
                        ) { Icon(Icons.AutoMirrored.Filled.Send, "发送追问", tint = OpsBlue) }
                    }
                )
                if (state.messages.any { it.role == "assistant" && it.content.isNotBlank() } && !state.running) {
                    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                        Text("本次诊断是否有帮助？", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp, modifier = Modifier.weight(1f))
                        IconButton(onClick = { onFeedback(true) }, modifier = Modifier.size(34.dp)) {
                            Icon(Icons.Default.ThumbUp, "诊断有帮助", tint = OpsGreen, modifier = Modifier.size(16.dp))
                        }
                        IconButton(onClick = { onFeedback(false) }, modifier = Modifier.size(34.dp)) {
                            Icon(Icons.Default.ThumbDown, "诊断无帮助", tint = OpsOrange, modifier = Modifier.size(16.dp))
                        }
                    }
                }
            }
        },
        containerColor = MaterialTheme.colorScheme.background
    )
}

@Composable
private fun DiagnosisDialog(report: InspectionReport, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        containerColor = MaterialTheme.colorScheme.surface,
        shape = RoundedCornerShape(16.dp),
        title = { Text("日志诊断结果", fontSize = 16.sp, fontWeight = FontWeight.Bold) },
        text = {
            LazyColumn(Modifier.heightIn(max = 480.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                item {
                    SelectionContainer {
                        MarkdownText(text = report.summary, color = MaterialTheme.colorScheme.onSurface)
                    }
                }
                items(report.findings) { finding ->
                    Card(colors = CardDefaults.cardColors(containerColor = severityColor(finding.severity).copy(alpha = 0.09f))) {
                        Column(Modifier.padding(10.dp)) {
                            StatusPill(finding.severity.uppercase(), severityColor(finding.severity))
                            Spacer(Modifier.height(5.dp))
                            Text(finding.title, fontWeight = FontWeight.Bold)
                            finding.detail?.let {
                                SelectionContainer {
                                    MarkdownText(text = it, color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 11.sp)
                                }
                            }
                        }
                    }
                }
                report.context?.let { item {
                    SelectionContainer {
                        MarkdownText(text = it, color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
                    }
                } }
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text("关闭") } }
    )
}

@Composable
private fun PortForwardPanel(
    rules: List<PortForwardRule>,
    loading: Boolean,
    hosts: List<Host>,
    httpProxies: List<HTTPProxyConfig>,
    busyIds: Set<String>,
    onRefresh: () -> Unit,
    onCreatePortForward: () -> Unit,
    onEditPortForward: (PortForwardRule) -> Unit,
    onDeletePortForward: (PortForwardRule) -> Unit,
    onTogglePortForward: (String, Boolean) -> Unit,
    onCreateHTTPProxy: () -> Unit,
    onEditHTTPProxy: (HTTPProxyConfig) -> Unit,
    onDeleteHTTPProxy: (HTTPProxyConfig) -> Unit,
    onToggleHTTPProxy: (String, Boolean) -> Unit
) {
    LaunchedEffect(Unit) { onRefresh() }
    val clipboardManager = LocalClipboardManager.current
    val totalCount = rules.size + httpProxies.size
    Column(Modifier.fillMaxSize()) {
        // 顶部操作栏：统一新增 + 刷新
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 8.dp),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Column {
                Text(
                    "全部转发规则（$totalCount）",
                    color = MaterialTheme.colorScheme.onSurface,
                    fontWeight = FontWeight.Bold,
                    fontSize = 14.sp
                )
                Text(
                    "TCP ${rules.count { it.protocol.equals("tcp", true) }} · UDP ${rules.count { it.protocol.equals("udp", true) }} · HTTP ${httpProxies.size}",
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    fontSize = 11.sp
                )
            }
            Row {
                var addMenuExpanded by remember { mutableStateOf(false) }
                Box {
                    ExtendedFloatingActionButton(
                        onClick = { addMenuExpanded = true },
                        icon = { Icon(Icons.Default.Add, null) },
                        text = { Text("新增") },
                        containerColor = OpsBlue,
                        contentColor = Color.White,
                        modifier = Modifier.height(40.dp)
                    )
                    DropdownMenu(expanded = addMenuExpanded, onDismissRequest = { addMenuExpanded = false }) {
                        DropdownMenuItem(
                            text = { Text("端口转发 (TCP/UDP)") },
                            onClick = { addMenuExpanded = false; onCreatePortForward() },
                            leadingIcon = { Icon(Icons.Default.Lan, null, tint = OpsBlue) }
                        )
                        DropdownMenuItem(
                            text = { Text("HTTP 代理") },
                            onClick = { addMenuExpanded = false; onCreateHTTPProxy() },
                            leadingIcon = { Icon(Icons.Default.Public, null, tint = OpsOrange) }
                        )
                    }
                }
                IconButton(onClick = onRefresh, enabled = !loading) {
                    Icon(Icons.Default.Refresh, "刷新", tint = OpsBlue)
                }
            }
        }
        if (loading) {
            Box(Modifier.fillMaxWidth().height(3.dp).background(OpsBlue.copy(alpha = 0.25f)))
        }
        if (totalCount == 0 && !loading) {
            Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Icon(Icons.Default.Lan, null, tint = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.size(48.dp))
                    Text("暂无转发规则", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 14.sp)
                    Text("点击上方按钮创建端口转发或 HTTP 代理", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 11.sp)
                }
            }
        } else {
            LazyColumn(
                Modifier.fillMaxSize().padding(horizontal = 14.dp),
                contentPadding = PaddingValues(bottom = 20.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                // TCP/UDP 转发规则
                if (rules.isNotEmpty()) {
                    item {
                        Text("端口转发", color = OpsBlue, fontWeight = FontWeight.Bold, fontSize = 12.sp, modifier = Modifier.padding(top = 4.dp))
                    }
                    items(rules, key = { "fwd:${it.id}" }) { rule ->
                        ForwardRuleCard(
                            rule = rule,
                            busy = "port-forward:${rule.id}" in busyIds,
                            onEdit = { onEditPortForward(rule) },
                            onDelete = { onDeletePortForward(rule) },
                            onToggle = { onTogglePortForward(rule.id, !rule.enabled) },
                            modifier = Modifier.animateItem()
                        )
                    }
                }
                // HTTP 代理
                if (httpProxies.isNotEmpty()) {
                    item {
                        Text("HTTP 代理", color = OpsOrange, fontWeight = FontWeight.Bold, fontSize = 12.sp, modifier = Modifier.padding(top = 4.dp))
                    }
                    items(httpProxies, key = { "http:${it.id}" }) { proxy ->
                        HTTPProxyCard(
                            proxy = proxy,
                            busy = "http-proxy:${proxy.id}" in busyIds,
                            onEdit = { onEditHTTPProxy(proxy) },
                            onDelete = { onDeleteHTTPProxy(proxy) },
                            onToggle = { onToggleHTTPProxy(proxy.id, !proxy.enabled) },
                            onCopyUrl = {
                                val baseUrl = ApiClient.buildProxyUrl(proxy.host_id, proxy.target_port, proxy.default_path)
                                val url = runCatching { ApiClient.fetchProxyToken() }
                                    .fold(
                                        onSuccess = { tok -> "$baseUrl?pt=$tok" },
                                        onFailure = { baseUrl }
                                    )
                                clipboardManager.setText(AnnotatedString(url))
                            },
                            modifier = Modifier.animateItem()
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun ForwardRuleCard(
    rule: PortForwardRule,
    busy: Boolean,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
    onToggle: () -> Unit,
    modifier: Modifier = Modifier
) {
    val protocolColor = if (rule.protocol.equals("udp", true)) OpsOrange else OpsBlue
    val isActive = rule.enabled && (rule.status.equals("active", true) || rule.status.equals("listening", true) || rule.status.isBlank())
    val statusColor = when {
        !rule.enabled -> Color(0xFF9E9E9E)
        rule.status.equals("error", true) -> OpsRed
        isActive -> OpsGreen
        else -> OpsBlue
    }
    val statusText = when {
        !rule.enabled -> "已停用"
        rule.status.equals("error", true) -> "异常"
        isActive -> "运行中"
        else -> rule.status.ifBlank { "运行中" }
    }
    Card(
        modifier = modifier,
        shape = RoundedCornerShape(14.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 2.dp)
    ) {
        Column(Modifier.fillMaxWidth().padding(14.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            // 第一行：协议标签 + 主机名 + 状态
            Row(verticalAlignment = Alignment.CenterVertically) {
                StatusPill(rule.protocol.uppercase().ifBlank { "TCP" }, protocolColor)
                Spacer(Modifier.width(8.dp))
                Text(
                    rule.hostname.ifBlank { rule.host_id.take(12) },
                    color = MaterialTheme.colorScheme.onSurface,
                    fontWeight = FontWeight.Bold,
                    modifier = Modifier.weight(1f),
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
                StatusDot(statusColor, 6.dp)
                Spacer(Modifier.width(4.dp))
                Text(statusText, color = statusColor, fontSize = 11.sp)
            }
            // 第二行：端口映射可视化
            Row(
                Modifier.fillMaxWidth().background(
                    MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.4f),
                    RoundedCornerShape(10.dp)
                ).padding(horizontal = 14.dp, vertical = 12.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.weight(1f)) {
                    Text("本地端口", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 9.sp)
                    Spacer(Modifier.height(2.dp))
                    Text(
                        "${rule.local_port}",
                        color = protocolColor,
                        fontWeight = FontWeight.Black,
                        fontSize = 20.sp,
                        fontFamily = FontFamily.Monospace
                    )
                }
                Box(
                    Modifier.width(40.dp).height(2.dp)
                        .background(statusColor.copy(alpha = if (isActive) 0.6f else 0.2f))
                )
                Icon(Icons.Default.ChevronRight, null, tint = statusColor.copy(alpha = if (isActive) 0.8f else 0.3f), modifier = Modifier.size(16.dp))
                Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.weight(1f)) {
                    Text("目标端口", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 9.sp)
                    Spacer(Modifier.height(2.dp))
                    Text(
                        "${rule.target_port}",
                        color = MaterialTheme.colorScheme.onSurface,
                        fontWeight = FontWeight.Black,
                        fontSize = 20.sp,
                        fontFamily = FontFamily.Monospace
                    )
                }
                Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.weight(0.7f)) {
                    Text("会话", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 9.sp)
                    Spacer(Modifier.height(2.dp))
                    Text(
                        "${rule.sessions}",
                        color = if (rule.sessions > 0) OpsGreen else MaterialTheme.colorScheme.onSurfaceVariant,
                        fontWeight = FontWeight.Bold,
                        fontSize = 16.sp,
                        fontFamily = FontFamily.Monospace
                    )
                }
            }
            // 第三行：监听地址 + 操作按钮
            Row(
                Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                if (rule.listen_addr.isNotBlank()) {
                    Text(
                        rule.listen_addr,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        fontSize = 10.sp,
                        fontFamily = FontFamily.Monospace,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f)
                    )
                } else {
                    Spacer(Modifier.weight(1f))
                }
                Row(horizontalArrangement = Arrangement.spacedBy(2.dp)) {
                    TextButton(onClick = onToggle, enabled = !busy) {
                        Text(if (rule.enabled) "停用" else "启用", color = if (rule.enabled) OpsOrange else OpsGreen, fontSize = 11.sp)
                    }
                    TextButton(onClick = onEdit, enabled = !busy) { Text("编辑", fontSize = 11.sp) }
                    TextButton(onClick = onDelete, enabled = !busy) { Text("删除", color = OpsRed, fontSize = 11.sp) }
                }
            }
        }
    }
}

@Composable
private fun HTTPProxyCard(
    proxy: HTTPProxyConfig,
    busy: Boolean,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
    onToggle: () -> Unit,
    onCopyUrl: suspend () -> Unit,
    modifier: Modifier = Modifier
) {
    val scope = rememberCoroutineScope()
    val statusColor = when {
        !proxy.enabled -> Color(0xFF9E9E9E)
        else -> OpsGreen
    }
    val statusText = if (proxy.enabled) "运行中" else "已停用"
    val proxyUrl = remember(proxy.host_id, proxy.target_port, proxy.default_path) {
        ApiClient.buildProxyUrl(proxy.host_id, proxy.target_port, proxy.default_path)
    }
    Card(
        modifier = modifier,
        shape = RoundedCornerShape(14.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 2.dp)
    ) {
        Column(Modifier.fillMaxWidth().padding(14.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            // 第一行：HTTP 标签 + 名称 + 状态
            Row(verticalAlignment = Alignment.CenterVertically) {
                StatusPill("HTTP", OpsOrange)
                Spacer(Modifier.width(8.dp))
                Text(
                    proxy.name.ifBlank { "${proxy.hostname}:${proxy.target_port}" },
                    color = MaterialTheme.colorScheme.onSurface,
                    fontWeight = FontWeight.Bold,
                    modifier = Modifier.weight(1f),
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
                StatusDot(statusColor, 6.dp)
                Spacer(Modifier.width(4.dp))
                Text(statusText, color = statusColor, fontSize = 11.sp)
            }
            // 第二行：代理信息可视化
            Row(
                Modifier.fillMaxWidth().background(
                    MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.4f),
                    RoundedCornerShape(10.dp)
                ).padding(horizontal = 14.dp, vertical = 12.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.weight(1f)) {
                    Text("目标主机", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 9.sp)
                    Spacer(Modifier.height(2.dp))
                    Text(
                        proxy.hostname.ifBlank { proxy.host_id.take(8) },
                        color = OpsBlue,
                        fontWeight = FontWeight.Bold,
                        fontSize = 13.sp,
                        fontFamily = FontFamily.Monospace,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                }
                Box(
                    Modifier.width(40.dp).height(2.dp)
                        .background(statusColor.copy(alpha = if (proxy.enabled) 0.6f else 0.2f))
                )
                Icon(Icons.Default.ChevronRight, null, tint = statusColor.copy(alpha = if (proxy.enabled) 0.8f else 0.3f), modifier = Modifier.size(16.dp))
                Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.weight(1f)) {
                    Text("目标端口", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 9.sp)
                    Spacer(Modifier.height(2.dp))
                    Text(
                        "${proxy.target_port}",
                        color = MaterialTheme.colorScheme.onSurface,
                        fontWeight = FontWeight.Black,
                        fontSize = 20.sp,
                        fontFamily = FontFamily.Monospace
                    )
                }
                Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.weight(0.7f)) {
                    Text("会话", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 9.sp)
                    Spacer(Modifier.height(2.dp))
                    Text(
                        "${proxy.sessions}",
                        color = if (proxy.sessions > 0) OpsGreen else MaterialTheme.colorScheme.onSurfaceVariant,
                        fontWeight = FontWeight.Bold,
                        fontSize = 16.sp,
                        fontFamily = FontFamily.Monospace
                    )
                }
            }
            // 第三行：转发地址 + 复制按钮
            Row(
                Modifier.fillMaxWidth().background(
                    OpsOrange.copy(alpha = 0.06f),
                    RoundedCornerShape(8.dp)
                ).clickable { scope.launch { onCopyUrl() } }.padding(horizontal = 10.dp, vertical = 8.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Icon(Icons.Default.Public, null, tint = OpsOrange, modifier = Modifier.size(14.dp))
                Spacer(Modifier.width(6.dp))
                Text(
                    proxyUrl,
                    color = MaterialTheme.colorScheme.onSurface,
                    fontSize = 10.sp,
                    fontFamily = FontFamily.Monospace,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f)
                )
                Spacer(Modifier.width(6.dp))
                Text(
                    "复制",
                    color = OpsOrange,
                    fontSize = 11.sp,
                    fontWeight = FontWeight.Bold,
                    modifier = Modifier.clickable { scope.launch { onCopyUrl() } }
                )
            }
            // 默认路径
            if (proxy.default_path.isNotBlank()) {
                Text(
                    "默认路径: ${proxy.default_path}",
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    fontSize = 10.sp,
                    fontFamily = FontFamily.Monospace
                )
            }
            // 操作按钮
            Row(
                Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.End,
                verticalAlignment = Alignment.CenterVertically
            ) {
                TextButton(onClick = onToggle, enabled = !busy) {
                    Text(if (proxy.enabled) "停用" else "启用", color = if (proxy.enabled) OpsOrange else OpsGreen, fontSize = 11.sp)
                }
                TextButton(onClick = onEdit, enabled = !busy) { Text("编辑", fontSize = 11.sp) }
                TextButton(onClick = onDelete, enabled = !busy) { Text("删除", color = OpsRed, fontSize = 11.sp) }
            }
        }
    }
}

@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
private fun PortForwardFormDialog(
    rule: PortForwardRule?,
    hosts: List<Host>,
    busy: Boolean,
    onDismiss: () -> Unit,
    onCreate: (PortForwardCreateRequest) -> Unit,
    onEdit: (String, PortForwardEditRequest) -> Unit
) {
    val isEdit = rule != null
    var selectedHostId by remember { mutableStateOf(rule?.host_id ?: hosts.firstOrNull()?.id ?: "") }
    var targetPort by remember { mutableStateOf(rule?.target_port?.toString() ?: "") }
    var localPort by remember { mutableStateOf(if (rule != null && rule.local_port > 0) rule.local_port.toString() else "") }
    var protocol by remember { mutableStateOf(rule?.protocol?.ifBlank { "tcp" } ?: "tcp") }

    val targetPortInt = targetPort.toIntOrNull() ?: 0
    val localPortInt = localPort.toIntOrNull() ?: 0
    val isValid = selectedHostId.isNotBlank() && targetPortInt in 1..65535 && (isEdit || localPortInt == 0 || localPortInt in 1..65535)

    AlertDialog(
        onDismissRequest = { if (!busy) onDismiss() },
        containerColor = MaterialTheme.colorScheme.surface,
        shape = RoundedCornerShape(16.dp),
        title = { Text(if (isEdit) "编辑端口转发规则" else "新增端口转发规则", fontSize = 16.sp, fontWeight = FontWeight.Bold) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                // 主机选择下拉
                if (hosts.isNotEmpty()) {
                    var expanded by remember { mutableStateOf(false) }
                    val selectedHost = hosts.find { it.id == selectedHostId }
                    ExposedDropdownMenuBox(expanded = expanded, onExpandedChange = { expanded = !expanded }) {
                        OutlinedTextField(
                            value = selectedHost?.let { "${it.hostname} (${it.ip ?: it.id.take(8)})" } ?: selectedHostId,
                            onValueChange = {},
                            readOnly = true,
                            modifier = Modifier.fillMaxWidth().menuAnchor(),
                            label = { Text("目标主机") },
                            trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded) }
                        )
                        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
                            hosts.sortedBy { it.hostname.lowercase() }.forEach { host ->
                                DropdownMenuItem(
                                    text = { Text("${host.hostname} (${host.ip ?: host.id.take(8)})") },
                                    onClick = {
                                        selectedHostId = host.id
                                        expanded = false
                                    }
                                )
                            }
                        }
                    }
                }
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedTextField(
                        targetPort, { targetPort = it.filter { c -> c.isDigit() } },
                        modifier = Modifier.weight(1f), label = { Text("目标端口") }, singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = androidx.compose.ui.text.input.KeyboardType.Number)
                    )
                    OutlinedTextField(
                        localPort, { localPort = it.filter { c -> c.isDigit() } },
                        modifier = Modifier.weight(1f), label = { Text("本地端口 (0=自动)") }, singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = androidx.compose.ui.text.input.KeyboardType.Number)
                    )
                }
                Row(horizontalArrangement = Arrangement.spacedBy(7.dp)) {
                    listOf("tcp" to "TCP", "udp" to "UDP").forEach { (key, label) ->
                        FilterChip(selected = protocol == key, onClick = { protocol = key }, label = { Text(label) })
                    }
                }
            }
        },
        confirmButton = {
            Button(
                onClick = {
                    if (isEdit) {
                        val editRule = rule ?: return@Button
                        onEdit(editRule.id, PortForwardEditRequest(
                            host_id = if (selectedHostId != editRule.host_id) selectedHostId else null,
                            target_port = targetPortInt,
                            local_port = localPortInt
                        ))
                    } else {
                        onCreate(PortForwardCreateRequest(
                            host_id = selectedHostId,
                            target_port = targetPortInt,
                            local_port = localPortInt,
                            protocol = protocol
                        ))
                    }
                },
                enabled = isValid && !busy
            ) {
                if (busy) CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp, color = MaterialTheme.colorScheme.onSurface)
                else Text("保存")
            }
        },
        dismissButton = { TextButton(onClick = onDismiss, enabled = !busy) { Text("取消") } }
    )
}

@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
private fun HTTPProxyFormDialog(
    proxy: HTTPProxyConfig?,
    hosts: List<Host>,
    busy: Boolean,
    onDismiss: () -> Unit,
    onCreate: (HTTPProxyConfig) -> Unit,
    onEdit: (String, HTTPProxyConfig) -> Unit
) {
    val isEdit = proxy != null
    var selectedHostId by remember { mutableStateOf(proxy?.host_id ?: hosts.firstOrNull()?.id ?: "") }
    var name by remember { mutableStateOf(proxy?.name ?: "") }
    var targetPort by remember { mutableStateOf(proxy?.target_port?.toString() ?: "") }
    var defaultPath by remember { mutableStateOf(proxy?.default_path ?: "") }

    val targetPortInt = targetPort.toIntOrNull() ?: 0
    val isValid = selectedHostId.isNotBlank() && targetPortInt in 1..65535

    AlertDialog(
        onDismissRequest = { if (!busy) onDismiss() },
        containerColor = MaterialTheme.colorScheme.surface,
        shape = RoundedCornerShape(16.dp),
        title = { Text(if (isEdit) "编辑 HTTP 代理" else "新增 HTTP 代理", fontSize = 16.sp, fontWeight = FontWeight.Bold) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                // 名称
                OutlinedTextField(
                    name, { name = it },
                    modifier = Modifier.fillMaxWidth(),
                    label = { Text("代理名称") },
                    singleLine = true,
                    placeholder = { Text("可选，如：内部 API 服务") }
                )
                // 主机选择下拉
                if (hosts.isNotEmpty()) {
                    var expanded by remember { mutableStateOf(false) }
                    val selectedHost = hosts.find { it.id == selectedHostId }
                    ExposedDropdownMenuBox(expanded = expanded, onExpandedChange = { expanded = !expanded }) {
                        OutlinedTextField(
                            value = selectedHost?.let { "${it.hostname} (${it.ip ?: it.id.take(8)})" } ?: selectedHostId,
                            onValueChange = {},
                            readOnly = true,
                            modifier = Modifier.fillMaxWidth().menuAnchor(),
                            label = { Text("目标主机") },
                            trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded) }
                        )
                        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
                            hosts.sortedBy { it.hostname.lowercase() }.forEach { host ->
                                DropdownMenuItem(
                                    text = { Text("${host.hostname} (${host.ip ?: host.id.take(8)})") },
                                    onClick = {
                                        selectedHostId = host.id
                                        expanded = false
                                    }
                                )
                            }
                        }
                    }
                }
                OutlinedTextField(
                    targetPort, { targetPort = it.filter { c -> c.isDigit() } },
                    modifier = Modifier.fillMaxWidth(), label = { Text("目标端口") }, singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = androidx.compose.ui.text.input.KeyboardType.Number)
                )
                OutlinedTextField(
                    defaultPath, { defaultPath = it },
                    modifier = Modifier.fillMaxWidth(), label = { Text("默认路径") }, singleLine = true,
                    placeholder = { Text("可选，如：/api/v1") }
                )
            }
        },
        confirmButton = {
            Button(
                onClick = {
                    val req = HTTPProxyConfig(
                        id = proxy?.id ?: "",
                        name = name,
                        host_id = selectedHostId,
                        target_port = targetPortInt,
                        default_path = defaultPath
                    )
                    if (isEdit) {
                        val editId = proxy?.id ?: return@Button
                        onEdit(editId, req)
                    } else {
                        onCreate(req)
                    }
                },
                enabled = isValid && !busy
            ) {
                if (busy) CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp, color = MaterialTheme.colorScheme.onSurface)
                else Text("保存")
            }
        },
        dismissButton = { TextButton(onClick = onDismiss, enabled = !busy) { Text("取消") } }
    )
}

private fun severityColor(value: String) = when (value.lowercase()) {
    "critical", "error" -> OpsRed
    "warning", "warn" -> OpsOrange
    else -> OpsBlue
}

private fun incidentStatusColor(value: String) = when (value) {
    "resolved" -> OpsGreen
    "acknowledged" -> OpsOrange
    else -> OpsRed
}

private fun logLevelColor(value: String) = when (value) {
    "error" -> OpsRed
    "warn" -> OpsOrange
    "debug" -> Color(0xFF8B5CF6)
    else -> OpsBlue
}

private fun executionStatusColor(value: String) = when (value) {
    "completed", "success" -> OpsGreen
    "failed", "timeout" -> OpsRed
    "running" -> OpsBlue
    else -> Color.Gray
}

private fun localizeIncidentStatus(value: String) = when (value) {
    "open" -> "待处理"
    "acknowledged" -> "已确认"
    "resolved" -> "已解决"
    else -> value
}

private fun localizeExecutionStatus(value: String) = when (value) {
    "running" -> "执行中"
    "completed" -> "已完成"
    "failed" -> "失败"
    "cancelled" -> "已取消"
    else -> value
}

private fun scheduleText(playbook: Playbook): String {
    val schedule = playbook.schedule ?: return "手动执行"
    if (!schedule.enabled) return "手动执行"
    return when (schedule.kind) {
        "interval" -> "每 ${schedule.interval_min} 分钟"
        "daily" -> "每天 ${schedule.at ?: "--:--"}"
        "weekly" -> "每周定时"
        else -> "定时执行"
    }
}

private fun formatOpsTime(timestamp: Long): String {
    if (timestamp <= 0) return "—"
    val millis = if (timestamp > 1_000_000_000_000L) timestamp else timestamp * 1000
    return SimpleDateFormat("MM-dd HH:mm:ss", Locale.getDefault()).format(Date(millis))
}
