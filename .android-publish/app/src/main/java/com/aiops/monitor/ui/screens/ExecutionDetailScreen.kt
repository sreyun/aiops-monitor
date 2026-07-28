package com.aiops.monitor.ui.screens

import androidx.compose.animation.animateContentSize
import androidx.compose.foundation.background
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.ExpandLess
import androidx.compose.material.icons.filled.ExpandMore
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavHostController
import com.aiops.monitor.data.models.HostExecutionResult
import com.aiops.monitor.data.models.PlaybookExecution
import com.aiops.monitor.data.models.PlaybookStepResult
import com.aiops.monitor.ui.components.LoadingBox
import com.aiops.monitor.ui.components.StateBox
import com.aiops.monitor.ui.components.StatusDot
import com.aiops.monitor.ui.components.StatusPill
import com.aiops.monitor.ui.viewmodel.ExecutionDetailViewModel
import kotlinx.coroutines.launch
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

private val ExecutionBlue = Color(0xFF4F7FFF)
private val ExecutionGreen = Color(0xFF00D68F)
private val ExecutionOrange = Color(0xFFF59E0B)
private val ExecutionRed = Color(0xFFFF5C63)

@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
fun ExecutionDetailScreen(navController: NavHostController, modifier: Modifier = Modifier) {
    val vm: ExecutionDetailViewModel = viewModel()
    val execution by vm.execution.collectAsState()
    val loading by vm.loading.collectAsState()
    val error by vm.error.collectAsState()
    val lastUpdated by vm.lastUpdated.collectAsState()
    val clipboard = LocalClipboardManager.current
    val snackbar = remember { SnackbarHostState() }
    val scope = rememberCoroutineScope()

    val copyOutput: (String, String) -> Unit = { text, label ->
        clipboard.setText(AnnotatedString(text))
        scope.launch { snackbar.showSnackbar("已复制$label") }
    }

    Scaffold(
        modifier = modifier,
        snackbarHost = { SnackbarHost(snackbar) },
        topBar = {
            TopAppBar(
                title = {
                    Column {
                        Text("剧本执行详情", fontWeight = FontWeight.Black, color = MaterialTheme.colorScheme.onSurface)
                        Text(
                            execution?.let { "#${it.id} · ${localizeExecutionStatus(it.status)}" }
                                ?: "读取执行实例",
                            style = MaterialTheme.typography.labelSmall,
                            color = execution?.let { executionColor(it.status) } ?: MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    }
                },
                navigationIcon = {
                    IconButton(onClick = navController::popBackStack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "返回")
                    }
                },
                actions = {
                    if (loading && execution != null) {
                        CircularProgressIndicator(Modifier.size(19.dp), strokeWidth = 2.dp, color = ExecutionBlue)
                        Spacer(Modifier.width(12.dp))
                    } else {
                        IconButton(onClick = vm::refresh) {
                            Icon(Icons.Default.Refresh, contentDescription = "刷新执行详情")
                        }
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.background,
                    titleContentColor = MaterialTheme.colorScheme.onSurface,
                    navigationIconContentColor = MaterialTheme.colorScheme.onSurface,
                    actionIconContentColor = MaterialTheme.colorScheme.onSurface
                )
            )
        },
        containerColor = MaterialTheme.colorScheme.background
    ) { padding ->
        when {
            execution == null && loading -> LoadingBox(Modifier.fillMaxSize().padding(padding))
            execution == null && error != null -> StateBox(
                error ?: "执行详情加载失败",
                Modifier.fillMaxSize().padding(padding),
                vm::refresh
            )
            execution != null -> {
                val exec = execution
                if (exec != null) {
                    ExecutionContent(
                        execution = exec,
                        lastUpdated = lastUpdated,
                        error = error,
                        modifier = Modifier.fillMaxSize().padding(padding),
                        onCopy = copyOutput
                    )
                }
            }
            else -> StateBox("执行实例不存在", Modifier.fillMaxSize().padding(padding), vm::refresh)
        }
    }
}

@Composable
private fun ExecutionContent(
    execution: PlaybookExecution,
    lastUpdated: Long,
    error: String?,
    modifier: Modifier,
    onCopy: (String, String) -> Unit
) {
    val results = (execution.host_results ?: emptyMap())
        .filterValues { it != null }
        .entries
        .sortedWith(
            compareBy<Map.Entry<String, HostExecutionResult>> { hostStatusRank(it.value.status) }
                .thenBy { it.value.hostname }
        )
    val successCount = results.count { it.value.status == "success" }
    val failedCount = results.count { it.value.status in setOf("failed", "timeout") }
    val pendingCount = results.size - successCount - failedCount
    val finishedCount = successCount + failedCount
    val progress = if (results.isEmpty()) 0f else finishedCount.toFloat() / results.size

    LazyColumn(
        modifier = modifier.padding(horizontal = 14.dp),
        contentPadding = PaddingValues(top = 10.dp, bottom = 28.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        item(key = "summary") {
            Card(
                shape = RoundedCornerShape(14.dp),
                colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface)
            ) {
                Column(Modifier.fillMaxWidth().padding(14.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        StatusDot(executionColor(execution.status), 9.dp)
                        Spacer(Modifier.width(8.dp))
                        Column(Modifier.weight(1f)) {
                            Text(execution.playbook_name, color = MaterialTheme.colorScheme.onSurface, fontWeight = FontWeight.Bold, fontSize = 17.sp)
                            Text(
                                "执行人 ${execution.operator.ifBlank { "system" }} · ${formatExecutionTime(execution.start_time)}",
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant
                            )
                        }
                        StatusPill(localizeExecutionStatus(execution.status), executionColor(execution.status))
                    }

                    LinearProgressIndicator(
                        progress = { progress },
                        modifier = Modifier.fillMaxWidth(),
                        color = executionColor(execution.status),
                        trackColor = MaterialTheme.colorScheme.outlineVariant
                    )

                    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                        ResultMetric("成功", successCount, ExecutionGreen)
                        ResultMetric("失败", failedCount, if (failedCount > 0) ExecutionRed else MaterialTheme.colorScheme.onSurfaceVariant)
                        ResultMetric("处理中", pendingCount, if (pendingCount > 0) ExecutionBlue else MaterialTheme.colorScheme.onSurfaceVariant)
                        ResultMetric("耗时", formatDuration(executionDurationSeconds(execution)), MaterialTheme.colorScheme.onSurface)
                    }

                    Text(
                        when {
                            execution.status == "running" -> "每 2 秒自动刷新，完成后会展示每台主机的步骤输出"
                            lastUpdated > 0 -> "最后同步 ${formatClock(lastUpdated)}"
                            else -> "执行结果已落库"
                        },
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                }
            }
        }

        error?.let {
            item(key = "error") {
                Card(colors = CardDefaults.cardColors(containerColor = ExecutionOrange.copy(alpha = 0.09f))) {
                    Text(it, color = ExecutionOrange, style = MaterialTheme.typography.bodySmall, modifier = Modifier.fillMaxWidth().padding(10.dp))
                }
            }
        }

        item(key = "host-heading") {
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Text("主机执行结果", style = MaterialTheme.typography.titleMedium, color = MaterialTheme.colorScheme.onSurface)
                Spacer(Modifier.weight(1f))
                Text("${results.size} 台", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }

        if (results.isEmpty()) {
            item(key = "empty") {
                Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface)) {
                    Column(
                        Modifier.fillMaxWidth().padding(26.dp),
                        horizontalAlignment = Alignment.CenterHorizontally,
                        verticalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        CircularProgressIndicator(Modifier.size(24.dp), strokeWidth = 2.dp, color = ExecutionBlue)
                        Text("等待目标主机接收任务", color = MaterialTheme.colorScheme.onSurface, fontWeight = FontWeight.Bold)
                        Text("Agent 接收任务后，这里会出现主机和步骤结果", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                }
            }
        } else {
            items(results, key = { it.key }) { (hostId, result) ->
                HostExecutionCard(hostId, result, onCopy)
            }
        }
    }
}

@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
private fun HostExecutionCard(
    hostId: String,
    result: HostExecutionResult,
    onCopy: (String, String) -> Unit
) {
    var expanded by rememberSaveable(hostId) { mutableStateOf(result.status in setOf("failed", "timeout")) }
    val color = executionColor(result.status)
    val stepList = result.steps ?: emptyList()
    val successSteps = stepList.count { it?.status == "success" }
    val failedSteps = stepList.count { it?.status == "failed" }

    LaunchedEffect(result.status) {
        if (result.status in setOf("failed", "timeout")) expanded = true
    }

    Card(
        onClick = { expanded = !expanded },
        modifier = Modifier.fillMaxWidth().animateContentSize(),
        shape = RoundedCornerShape(12.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface)
    ) {
        Column(Modifier.fillMaxWidth().padding(12.dp), verticalArrangement = Arrangement.spacedBy(9.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                StatusDot(color, 8.dp)
                Spacer(Modifier.width(8.dp))
                Column(Modifier.weight(1f)) {
                    Text(result.hostname.ifBlank { hostId }, color = MaterialTheme.colorScheme.onSurface, fontWeight = FontWeight.Bold, maxLines = 1)
                    Text(
                        "$hostId · ${stepList.size} 步 · 成功 $successSteps${if (failedSteps > 0) " · 失败 $failedSteps" else ""}",
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                }
                StatusPill(localizeHostStatus(result.status), color)
                Icon(
                    if (expanded) Icons.Default.ExpandLess else Icons.Default.ExpandMore,
                    contentDescription = if (expanded) "收起主机结果" else "展开主机结果",
                    tint = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }

            if (expanded) {
                HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                if (stepList.isEmpty()) {
                    OutputBlock("主机输出", result.output ?: "", onCopy)
                } else {
                    stepList.filterNotNull().forEachIndexed { index, step ->
                        StepResultCard(index + 1, step, onCopy)
                    }
                }
            }
        }
    }
}

@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
private fun StepResultCard(index: Int, step: PlaybookStepResult, onCopy: (String, String) -> Unit) {
    var expanded by rememberSaveable(step.name, index) { mutableStateOf(step.status == "failed") }
    val color = executionColor(step.status)
    Card(
        onClick = { expanded = !expanded },
        modifier = Modifier.fillMaxWidth().animateContentSize(),
        shape = RoundedCornerShape(9.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.background)
    ) {
        Column(Modifier.fillMaxWidth().padding(10.dp), verticalArrangement = Arrangement.spacedBy(7.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("$index", color = color, fontFamily = FontFamily.Monospace, fontWeight = FontWeight.Black)
                Spacer(Modifier.width(8.dp))
                Text(step.name.ifBlank { "未命名步骤" }, color = MaterialTheme.colorScheme.onSurface, fontWeight = FontWeight.Medium, modifier = Modifier.weight(1f))
                Text(formatMillis(step.duration_ms), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                Spacer(Modifier.width(7.dp))
                StatusPill(localizeStepStatus(step.status), color)
            }
            if (expanded) OutputBlock("步骤输出", step.output ?: "", onCopy)
        }
    }
}

@Composable
private fun OutputBlock(title: String, rawOutput: String, onCopy: (String, String) -> Unit) {
    val output = rawOutput ?: ""
    val truncated = output.length > 50_000
    val displayOutput = remember(output) { if (truncated) output.takeLast(50_000) else output }
    Column(verticalArrangement = Arrangement.spacedBy(5.dp)) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Text(title, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.weight(1f))
            if (output.isNotBlank()) {
                TextButton(onClick = { onCopy(output, title) }, contentPadding = PaddingValues(horizontal = 8.dp, vertical = 0.dp)) {
                    Icon(Icons.Default.ContentCopy, contentDescription = null, modifier = Modifier.size(15.dp))
                    Spacer(Modifier.width(4.dp))
                    Text("复制")
                }
            }
        }
        Box(
            Modifier.fillMaxWidth().heightIn(min = 48.dp, max = 260.dp)
                .background(Color.Black.copy(alpha = 0.42f), RoundedCornerShape(7.dp))
                .padding(9.dp)
                .verticalScroll(rememberScrollState())
                .horizontalScroll(rememberScrollState())
        ) {
            SelectionContainer {
                Text(
                    displayOutput.ifBlank { "（无标准输出）" },
                    color = if (output.isBlank()) MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.onSurface,
                    fontFamily = FontFamily.Monospace,
                    fontSize = 11.sp,
                    lineHeight = 17.sp,
                    softWrap = false
                )
            }
        }
        if (truncated) {
            Text("输出过长，界面显示末尾 50,000 字符；复制按钮仍会复制完整内容", style = MaterialTheme.typography.labelSmall, color = ExecutionOrange)
        }
    }
}

@Composable
private fun ResultMetric(label: String, value: Any, color: Color) {
    Column(horizontalAlignment = Alignment.CenterHorizontally) {
        Text(value.toString(), color = color, fontWeight = FontWeight.Bold, fontSize = 15.sp)
        Text(label, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
private fun executionColor(status: String): Color = when (status.lowercase()) {
    "completed", "success" -> ExecutionGreen
    "failed", "timeout" -> ExecutionRed
    "running", "pending" -> ExecutionBlue
    "skipped" -> ExecutionOrange
    else -> MaterialTheme.colorScheme.onSurfaceVariant
}

private fun hostStatusRank(status: String): Int = when (status.lowercase()) {
    "failed", "timeout" -> 0
    "running", "pending" -> 1
    else -> 2
}

private fun localizeExecutionStatus(status: String): String = when (status.lowercase()) {
    "running" -> "执行中"
    "completed" -> "已完成"
    "failed" -> "执行失败"
    "cancelled" -> "已取消"
    else -> status
}

private fun localizeHostStatus(status: String): String = when (status.lowercase()) {
    "pending" -> "等待中"
    "running" -> "执行中"
    "success" -> "成功"
    "failed" -> "失败"
    "timeout" -> "超时"
    else -> status
}

private fun localizeStepStatus(status: String): String = when (status.lowercase()) {
    "pending" -> "等待"
    "running" -> "执行中"
    "success" -> "成功"
    "failed" -> "失败"
    "skipped" -> "跳过"
    else -> status
}

private fun executionDurationSeconds(execution: PlaybookExecution): Long {
    val end = execution.end_time.takeIf { it > 0 } ?: (System.currentTimeMillis() / 1000)
    return (end - execution.start_time).coerceAtLeast(0)
}

private fun formatDuration(seconds: Long): String = when {
    seconds >= 3600 -> "%dh%02dm".format(seconds / 3600, seconds % 3600 / 60)
    seconds >= 60 -> "%dm%02ds".format(seconds / 60, seconds % 60)
    else -> "${seconds}s"
}

private fun formatMillis(millis: Long): String = when {
    millis >= 60_000 -> "%.1f min".format(millis / 60_000.0)
    millis >= 1_000 -> "%.1f s".format(millis / 1_000.0)
    else -> "${millis} ms"
}

private fun formatExecutionTime(timestamp: Long): String {
    if (timestamp <= 0) return "—"
    val millis = if (timestamp > 1_000_000_000_000L) timestamp else timestamp * 1000
    return SimpleDateFormat("MM-dd HH:mm:ss", Locale.getDefault()).format(Date(millis))
}

private fun formatClock(timestampMillis: Long): String =
    SimpleDateFormat("HH:mm:ss", Locale.getDefault()).format(Date(timestampMillis))
