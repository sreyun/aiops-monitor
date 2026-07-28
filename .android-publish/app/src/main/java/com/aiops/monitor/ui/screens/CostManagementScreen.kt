package com.aiops.monitor.ui.screens

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
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
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.navigation.NavHostController
import com.aiops.monitor.data.ApiClient
import com.aiops.monitor.data.models.AiCallStat
import com.aiops.monitor.data.models.AiStatsResponse
import com.aiops.monitor.data.models.AiUsageHistoryPoint
import com.aiops.monitor.data.models.AiUserUsageRow
import com.aiops.monitor.ui.components.HistoryChartCard
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/**
 * 成本管理：同步 Web「AI 调用观测」的调用次数 / 延迟 / Token / 费用。
 * 数据来自 PostgreSQL 永久落库接口。
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun CostManagementScreen(
    navController: NavHostController,
    modifier: Modifier = Modifier,
) {
    val scope = rememberCoroutineScope()
    var days by remember { mutableIntStateOf(7) }
    var loading by remember { mutableStateOf(true) }
    var error by remember { mutableStateOf<String?>(null) }
    var stats by remember { mutableStateOf<AiStatsResponse?>(null) }
    var points by remember { mutableStateOf<List<AiUsageHistoryPoint>>(emptyList()) }
    var users by remember { mutableStateOf<List<AiUserUsageRow>>(emptyList()) }
    var currency by remember { mutableStateOf("CNY") }

    fun reload() {
        scope.launch {
            loading = true
            error = null
            try {
                val to = System.currentTimeMillis() / 1000
                val from = to - days.toLong() * 86400
                val (s, h, u) = withContext(Dispatchers.IO) {
                    Triple(
                        ApiClient.api.aiStats(days),
                        ApiClient.api.aiUsageHistory(from, to),
                        ApiClient.api.aiUsageByUser(from, to, 15),
                    )
                }
                stats = s
                points = h.points.orEmpty()
                users = u.users.orEmpty()
                currency = s.cost_currency?.takeIf { it.isNotBlank() }
                    ?: h.cost_currency?.takeIf { it.isNotBlank() }
                    ?: u.cost_currency?.takeIf { it.isNotBlank() }
                    ?: "CNY"
            } catch (e: Exception) {
                error = e.message ?: "加载失败"
            } finally {
                loading = false
            }
        }
    }

    LaunchedEffect(days) { reload() }

    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Text("成本管理", fontWeight = FontWeight.Bold, style = MaterialTheme.typography.titleMedium)
                },
                navigationIcon = {
                    IconButton(onClick = { navController.popBackStack() }) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "返回")
                    }
                },
                actions = {
                    IconButton(onClick = { reload() }, enabled = !loading) {
                        Icon(Icons.Default.Refresh, contentDescription = "刷新")
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background)
            )
        },
        containerColor = MaterialTheme.colorScheme.background
    ) { padding ->
        Column(
            modifier
                .fillMaxSize()
                .padding(padding)
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(14.dp)
        ) {
            Text(
                "调用次数 · 延迟 · Token · 费用（与 Web AI 观测同步，PostgreSQL 永久保存）",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )

            Row(
                Modifier.horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                listOf(1 to "近 24h", 7 to "近 7 天", 30 to "近 30 天", 90 to "近 90 天").forEach { (d, label) ->
                    FilterChip(
                        selected = days == d,
                        onClick = { days = d },
                        label = { Text(label) }
                    )
                }
            }

            when {
                loading && stats == null -> {
                    Box(Modifier.fillMaxWidth().height(160.dp), contentAlignment = Alignment.Center) {
                        CircularProgressIndicator(modifier = Modifier.size(28.dp))
                    }
                }
                error != null && stats == null -> {
                    Text(error ?: "加载失败", color = Color(0xFFEF4444), style = MaterialTheme.typography.bodyMedium)
                }
                else -> {
                    val s = stats ?: AiStatsResponse()
                    val rate = if (s.total > 0) String.format(Locale.US, "%.1f", s.fail_rate * 100) else "0.0"
                    val metrics = listOf(
                        "调用次数" to "${s.total}",
                        "失败率" to "$rate%",
                        "平均延迟" to "${s.avg_latency_ms} ms",
                        "Token" to "${s.approx_tokens_total}",
                        "估算费用" to String.format(Locale.US, "%.4f %s", s.cost_total, currency),
                        "存储" to if (s.persisted) "PostgreSQL" else "进程内",
                    )
                    metrics.chunked(2).forEach { row ->
                        Row(
                            Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.spacedBy(10.dp)
                        ) {
                            row.forEach { (label, value) ->
                                MetricTile(label, value, Modifier.weight(1f))
                            }
                            if (row.size == 1) Spacer(Modifier.weight(1f))
                        }
                    }

                    if (points.isNotEmpty()) {
                        val ts = points.map { it.timestamp }
                        Text("历史趋势", fontWeight = FontWeight.SemiBold, style = MaterialTheme.typography.titleSmall)
                        HistoryChartCard(
                            title = "调用次数",
                            values = points.map { it.calls.toDouble() },
                            timestamps = ts,
                            unit = "次",
                            color = Color(0xFF4C8DFF),
                            onExpand = {}
                        )
                        HistoryChartCard(
                            title = "Token",
                            values = points.map { it.tokens.toDouble() },
                            timestamps = ts,
                            unit = "tok",
                            color = Color(0xFF22C55E),
                            onExpand = {}
                        )
                        HistoryChartCard(
                            title = "平均延迟",
                            values = points.map { it.avg_latency_ms.toDouble() },
                            timestamps = ts,
                            unit = "ms",
                            color = Color(0xFFEAB308),
                            onExpand = {}
                        )
                        HistoryChartCard(
                            title = "费用 ($currency)",
                            values = points.map { it.cost },
                            timestamps = ts,
                            unit = currency,
                            color = Color(0xFFF97316),
                            onExpand = {}
                        )
                    } else {
                        Text(
                            "暂无历史曲线（完成若干 AI 调用后出现）",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    }

                    if (users.isNotEmpty()) {
                        SectionCard(title = "用户成本排行") {
                            users.forEach { u ->
                                Row(
                                    Modifier.fillMaxWidth().padding(vertical = 6.dp),
                                    horizontalArrangement = Arrangement.SpaceBetween
                                ) {
                                    Column(Modifier.weight(1f)) {
                                        Text(u.actor.ifBlank { "(system)" }, fontWeight = FontWeight.Medium, fontSize = 13.sp)
                                        Text(
                                            "${u.calls} 次 · ${u.tokens} tok",
                                            fontSize = 11.sp,
                                            color = MaterialTheme.colorScheme.onSurfaceVariant
                                        )
                                    }
                                    Text(
                                        String.format(Locale.US, "%.4f %s", u.cost, currency),
                                        fontFamily = FontFamily.Monospace,
                                        fontSize = 12.sp,
                                        color = Color(0xFFF97316)
                                    )
                                }
                            }
                        }
                    }

                    val byTask = s.by_task.orEmpty()
                    if (byTask.isNotEmpty()) {
                        SectionCard(title = "按任务") {
                            byTask.toList().sortedByDescending { it.second.count }.forEach { (task, agg) ->
                                Row(
                                    Modifier.fillMaxWidth().padding(vertical = 5.dp),
                                    horizontalArrangement = Arrangement.SpaceBetween
                                ) {
                                    Text(task.ifBlank { "(unknown)" }, fontFamily = FontFamily.Monospace, fontSize = 12.sp, modifier = Modifier.weight(1f))
                                    Text("${agg.count} / 失败 ${agg.fail} / ${agg.avg_ms}ms", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                }
                            }
                        }
                    }

                    val recent = s.recent.orEmpty()
                    if (recent.isNotEmpty()) {
                        SectionCard(title = "最近调用") {
                            recent.take(10).forEach { RecentCallRow(it) }
                        }
                    }

                    error?.let {
                        Text(it, color = Color(0xFFEF4444), style = MaterialTheme.typography.bodySmall)
                    }
                }
            }
        }
    }
}

@Composable
private fun MetricTile(label: String, value: String, modifier: Modifier = Modifier) {
    Card(
        modifier = modifier,
        shape = RoundedCornerShape(12.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 0.dp)
    ) {
        Column(Modifier.padding(horizontal = 12.dp, vertical = 10.dp)) {
            Text(label, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            Spacer(Modifier.height(4.dp))
            Text(value, fontWeight = FontWeight.Bold, fontSize = 15.sp, color = MaterialTheme.colorScheme.onSurface)
        }
    }
}

@Composable
private fun SectionCard(title: String, content: @Composable () -> Unit) {
    Card(
        shape = RoundedCornerShape(14.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 0.dp)
    ) {
        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(title, fontWeight = FontWeight.SemiBold, style = MaterialTheme.typography.titleSmall)
            content()
        }
    }
}

@Composable
private fun RecentCallRow(r: AiCallStat) {
    val df = remember { SimpleDateFormat("MM-dd HH:mm", Locale.getDefault()) }
    val time = if (r.ts > 0) df.format(Date(r.ts * 1000)) else "-"
    val okColor = if (r.ok) Color(0xFF22C55E) else Color(0xFFEF4444)
    Column(Modifier.padding(vertical = 4.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(if (r.ok) "OK" else "FAIL", color = okColor, fontWeight = FontWeight.Bold, fontSize = 11.sp)
            Spacer(Modifier.width(8.dp))
            Text(r.task.ifBlank { "-" }, fontFamily = FontFamily.Monospace, fontSize = 12.sp)
            Spacer(Modifier.weight(1f))
            Text(time, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        Text(
            "${r.latency_ms}ms · ≈${r.approx_tokens} tok" +
                (if (r.actor.isNotBlank()) " · ${r.actor}" else "") +
                (if (r.error.isNotBlank()) " · ${r.error}" else ""),
            fontSize = 11.sp,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
    }
}
