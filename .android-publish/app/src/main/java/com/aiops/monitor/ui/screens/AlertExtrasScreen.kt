package com.aiops.monitor.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavHostController
import com.aiops.monitor.data.models.AlertGovernance
import com.aiops.monitor.data.models.AlertMatch
import com.aiops.monitor.data.models.AlertRecord
import com.aiops.monitor.ui.components.*
import com.aiops.monitor.ui.viewmodel.AlertsViewModel
import kotlinx.coroutines.launch

private val AeRed = Color(0xFFEF4444)
private val AeOrange = Color(0xFFF59E0B)
private val AeGreen = Color(0xFF00A86B)
private val AeBlue = Color(0xFF4F7FFF)
private val AeGray = Color(0xFF8A93A3)

private fun aeLevelColor(l: String) = when (l.lowercase()) { "critical" -> AeRed; "warning" -> AeOrange; else -> AeBlue }
private fun aeLevelLabel(l: String) = when (l.lowercase()) { "critical" -> "严重"; "warning" -> "警告"; else -> l }

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AlertExtrasScreen(navController: NavHostController, modifier: Modifier = Modifier) {
    val vm: AlertsViewModel = viewModel()
    val history by vm.history.collectAsState()
    val governance by vm.governance.collectAsState()
    var seg by remember { mutableIntStateOf(0) } // 0 历史 / 1 治理

    LaunchedEffect(Unit) { vm.loadHistory(); vm.loadGovernance() }

    Scaffold(
        modifier = modifier,
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            Column {
                TopAppBar(
                    title = { Text("告警历史 · 治理", fontWeight = FontWeight.Bold) },
                    navigationIcon = {
                        IconButton(onClick = { navController.popBackStack() }) {
                            Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回", tint = MaterialTheme.colorScheme.onSurface)
                        }
                    },
                    colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background)
                )
                Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 6.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    listOf("历史", "治理规则").forEachIndexed { i, label ->
                        FilterChip(selected = seg == i, onClick = { seg = i }, label = { Text(label, fontWeight = FontWeight.Medium) }, colors = chipColors())
                    }
                }
            }
        }
    ) { padding ->
        Box(Modifier.fillMaxSize().padding(padding)) {
            if (seg == 0) HistoryList(history) else GovernanceList(governance, vm)
        }
    }
}

@Composable
private fun HistoryList(history: List<AlertRecord>) {
    if (history.isEmpty()) { StateBox("暂无历史告警", Modifier.fillMaxSize()); return }
    LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        items(history, key = { it.id }) { r ->
            val resolved = r.resolved_at > 0
            Card(Modifier.fillMaxWidth(), shape = RoundedCornerShape(12.dp), colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface), elevation = CardDefaults.cardElevation(defaultElevation = 1.dp)) {
                Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        StatusPill(aeLevelLabel(r.level), aeLevelColor(r.level))
                        Text("${r.hostname} · ${r.type.uppercase()}", fontSize = 13.sp, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface, modifier = Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis)
                        StatusPill(if (resolved) "已恢复" else "触发中", if (resolved) AeGreen else AeRed)
                    }
                    if (r.message.isNotBlank()) Text(r.message, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 2, overflow = TextOverflow.Ellipsis)
                    Text(
                        "触发 ${fmtEpoch(r.fired_at)}" + if (resolved) " · 恢复 ${fmtEpoch(r.resolved_at)}" else "",
                        fontSize = 10.sp, color = AeGray
                    )
                }
            }
        }
    }
}

@Composable
private fun GovernanceList(gov: AlertGovernance?, vm: AlertsViewModel) {
    val silence = gov?.silence_rules.orEmpty()
    val inhibit = gov?.inhibit_rules.orEmpty()
    val routes = gov?.routes.orEmpty()
    val snack = remember { SnackbarHostState() }
    val scope = rememberCoroutineScope()
    if (gov == null || (silence.isEmpty() && inhibit.isEmpty() && routes.isEmpty())) {
        StateBox("暂无治理规则\n可在 Web 端新建后，在此开关启用状态", Modifier.fillMaxSize()); return
    }
    Box(Modifier.fillMaxSize()) {
        LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            item {
                Text(
                    "可在此启用/停用规则；复杂编辑请用 Web。保存会整体提交治理配置。",
                    fontSize = 11.sp,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }
            if (silence.isNotEmpty()) {
                item { GovHeader("静默规则", silence.size) }
                itemsIndexed(silence, key = { _, r -> "sil-${r.id}" }) { _, r ->
                    val win = if (r.time_start.isNotBlank()) "${r.time_start}-${r.time_end}" else "全天"
                    GovCard(
                        name = r.name,
                        enabled = r.enabled,
                        summary = listOf(matchSummary(r.match), "时段 $win").filter { it.isNotBlank() }.joinToString(" · "),
                        onToggle = { en ->
                            vm.toggleSilenceEnabled(r.id, en) { ok, err ->
                                scope.launch { snack.showSnackbar(if (ok) "已保存" else (err ?: "失败")) }
                            }
                        }
                    )
                }
            }
            if (inhibit.isNotEmpty()) {
                item { GovHeader("抑制规则", inhibit.size) }
                itemsIndexed(inhibit, key = { _, r -> "inh-${r.id}" }) { _, r ->
                    GovCard(
                        name = r.name,
                        enabled = r.enabled,
                        summary = "源[${matchSummary(r.source)}] → 目标[${matchSummary(r.target)}]" + if (r.same_host) " · 同主机" else "",
                        onToggle = { en ->
                            vm.toggleInhibitEnabled(r.id, en) { ok, err ->
                                scope.launch { snack.showSnackbar(if (ok) "已保存" else (err ?: "失败")) }
                            }
                        }
                    )
                }
            }
            if (routes.isNotEmpty()) {
                item { GovHeader("通知路由", routes.size) }
                itemsIndexed(routes, key = { _, r -> "rt-${r.id}" }) { _, r ->
                    GovCard(
                        name = r.name,
                        enabled = r.enabled,
                        summary = listOf(matchSummary(r.match), "渠道 ${r.channels.orEmpty().joinToString("/").ifBlank { "—" }}").filter { it.isNotBlank() }.joinToString(" · "),
                        onToggle = { en ->
                            vm.toggleRouteEnabled(r.id, en) { ok, err ->
                                scope.launch { snack.showSnackbar(if (ok) "已保存" else (err ?: "失败")) }
                            }
                        }
                    )
                }
            }
        }
        SnackbarHost(snack, modifier = Modifier.align(Alignment.BottomCenter))
    }
}

@Composable
private fun GovHeader(title: String, count: Int) {
    Text("$title · $count", fontWeight = FontWeight.Bold, fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurface, modifier = Modifier.padding(top = 4.dp))
}

@Composable
private fun GovCard(name: String, enabled: Boolean, summary: String, onToggle: (Boolean) -> Unit) {
    Card(Modifier.fillMaxWidth(), shape = RoundedCornerShape(12.dp), colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface), elevation = CardDefaults.cardElevation(defaultElevation = 1.dp)) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(name.ifBlank { "未命名规则" }, fontSize = 13.sp, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface, modifier = Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis)
                Switch(checked = enabled, onCheckedChange = onToggle)
            }
            if (summary.isNotBlank()) Text(summary, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

private fun matchSummary(m: AlertMatch?): String {
    if (m == null) return "全部"
    val parts = buildList {
        if (m.host_pattern.isNotBlank()) add("主机~${m.host_pattern}")
        m.types?.takeIf { it.isNotEmpty() }?.let { add(it.joinToString("/")) }
        m.levels?.takeIf { it.isNotEmpty() }?.let { add(it.joinToString("/")) }
    }
    return if (parts.isEmpty()) "全部" else parts.joinToString(" ")
}

private fun fmtEpoch(sec: Long): String {
    if (sec <= 0) return "-"
    val millis = if (sec > 1_000_000_000_000L) sec else sec * 1000
    return java.text.SimpleDateFormat("MM-dd HH:mm", java.util.Locale.getDefault()).format(java.util.Date(millis))
}
