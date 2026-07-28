@file:OptIn(ExperimentalFoundationApi::class)
package com.aiops.monitor.ui.screens

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavHostController
import com.aiops.monitor.data.ApiClient
import com.aiops.monitor.data.SessionTicker
import com.aiops.monitor.data.models.Host
import com.aiops.monitor.ui.Routes
import com.aiops.monitor.ui.components.*
import com.aiops.monitor.ui.formatBytes
import com.aiops.monitor.ui.viewmodel.HostsViewModel
import kotlinx.coroutines.delay

enum class HostQuickFilter { All, Online }

/**
 * 主机列表内容（基础设施中枢「主机」子标签）。无 Scaffold/TopAppBar，供 InfraHubScreen 内嵌。
 * 承接原总览页的 Agent 主机监控：搜索 + 分类 + 在线筛选 + 主机卡片 + GPU 概览。
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HostListContent(navController: NavHostController, modifier: Modifier = Modifier, refreshSignal: Int = 0) {
    val vm: HostsViewModel = viewModel()
    val hosts by vm.hosts.collectAsState()
    val loading by vm.loading.collectAsState()
    val error by vm.error.collectAsState()
    val listState = rememberLazyListState()

    var searchQuery by remember { mutableStateOf("") }
    var selectedCategory by remember { mutableStateOf<String?>(null) }
    var hostQuickFilter by remember { mutableStateOf(HostQuickFilter.All) }

    LaunchedEffect(Unit) {
        SessionTicker.pollWhileAlive(10_000L) { vm.load() }
    }
    LaunchedEffect(refreshSignal) { if (refreshSignal > 0) vm.load() }

    val filteredHosts = remember(hosts, searchQuery, selectedCategory, hostQuickFilter) {
        hosts.filter {
            (searchQuery.isEmpty() || it.hostname.contains(searchQuery, ignoreCase = true) || it.ip?.contains(searchQuery) == true) &&
            (selectedCategory == null || it.category == selectedCategory) &&
            (hostQuickFilter == HostQuickFilter.All || it.online)
        }.sortedWith(
            compareBy<Host> { if (it.online) 0 else 1 }
                .thenBy { it.hostname.lowercase() }
        )
    }
    val categories = remember(hosts) { hosts.mapNotNull { it.category }.distinct().sorted() }

    when {
        error != null && hosts.isEmpty() -> StateBox(message = error ?: "同步错误", modifier = modifier.fillMaxSize(), onRetry = vm::load)
        loading && hosts.isEmpty() -> LoadingBox(modifier.fillMaxSize())
        else -> LazyColumn(
            state = listState,
            modifier = modifier.fillMaxSize().padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
            contentPadding = PaddingValues(top = 12.dp, bottom = 24.dp)
        ) {
            item(key = "search") {
                OutlinedTextField(
                    value = searchQuery,
                    onValueChange = { searchQuery = it },
                    modifier = Modifier.fillMaxWidth(),
                    placeholder = { Text("搜索主机名或 IP", color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.6f)) },
                    leadingIcon = { Icon(Icons.Default.Search, contentDescription = null, tint = MaterialTheme.colorScheme.onSurfaceVariant) },
                    trailingIcon = if (searchQuery.isNotEmpty()) {
                        {
                            IconButton(onClick = { searchQuery = "" }) {
                                Icon(Icons.Default.Close, contentDescription = "清空搜索", tint = MaterialTheme.colorScheme.onSurfaceVariant)
                            }
                        }
                    } else null,
                    singleLine = true,
                    shape = RoundedCornerShape(12.dp),
                    colors = OutlinedTextFieldDefaults.colors(
                        focusedBorderColor = Color(0xFF4F7FFF),
                        unfocusedBorderColor = MaterialTheme.colorScheme.outlineVariant,
                        focusedTextColor = MaterialTheme.colorScheme.onSurface,
                        unfocusedTextColor = MaterialTheme.colorScheme.onSurface,
                        focusedContainerColor = MaterialTheme.colorScheme.surface,
                        unfocusedContainerColor = MaterialTheme.colorScheme.surface,
                        cursorColor = Color(0xFF4F7FFF)
                    )
                )
            }

            item(key = "quickfilter") {
                Row(
                    Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    FilterChip(
                        selected = selectedCategory == null && hostQuickFilter == HostQuickFilter.All,
                        onClick = { selectedCategory = null; hostQuickFilter = HostQuickFilter.All },
                        label = { Text("全部 ${hosts.size}") },
                        colors = chipColors()
                    )
                    FilterChip(
                        selected = hostQuickFilter == HostQuickFilter.Online,
                        onClick = { hostQuickFilter = if (hostQuickFilter == HostQuickFilter.Online) HostQuickFilter.All else HostQuickFilter.Online },
                        label = { Text("仅在线 ${hosts.count { it.online }}") },
                        colors = chipColors()
                    )
                    categories.forEach { cat ->
                        val count = hosts.count { it.category == cat }
                        FilterChip(
                            selected = selectedCategory == cat,
                            onClick = { selectedCategory = if (selectedCategory == cat) null else cat },
                            label = { Text("$cat $count") },
                            colors = chipColors()
                        )
                    }
                }
            }

            if (hosts.any { !it.latest?.gpus.isNullOrEmpty() }) {
                item(key = "gpu-overview") { GpuOverviewCard(hosts) { hostId -> navController.navigate(Routes.hostDetail(hostId)) } }
            }

            item(key = "host-title") {
                Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                    Text("主机 ${filteredHosts.size}/${hosts.size}", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface)
                    Spacer(Modifier.weight(1f))
                    if (loading) CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp, color = Color(0xFF4F7FFF))
                }
            }

            if (filteredHosts.isEmpty()) {
                item(key = "empty") {
                    Card(
                        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
                        elevation = CardDefaults.cardElevation(defaultElevation = 1.dp),
                        shape = RoundedCornerShape(12.dp)
                    ) {
                        Column(
                            Modifier.fillMaxWidth().padding(32.dp),
                            horizontalAlignment = Alignment.CenterHorizontally,
                            verticalArrangement = Arrangement.spacedBy(12.dp)
                        ) {
                            Icon(Icons.Default.Search, contentDescription = null, tint = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.size(36.dp))
                            Text("没有匹配的主机", color = MaterialTheme.colorScheme.onSurface, fontWeight = FontWeight.Bold)
                            TextButton(onClick = { searchQuery = ""; selectedCategory = null; hostQuickFilter = HostQuickFilter.All }) { Text("清除筛选") }
                        }
                    }
                }
            } else {
                items(filteredHosts, key = { it.id }) { host ->
                    HostCard(host, Modifier.animateItem()) { navController.navigate(Routes.hostDetail(host.id)) }
                }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun chipColors() = FilterChipDefaults.filterChipColors(
    containerColor = Color.Transparent,
    labelColor = MaterialTheme.colorScheme.onSurfaceVariant,
    selectedContainerColor = Color(0xFF4F7FFF).copy(alpha = 0.2f),
    selectedLabelColor = Color(0xFF4F7FFF),
    selectedLeadingIconColor = Color(0xFF4F7FFF)
)

@Composable
fun HostCard(host: Host, modifier: Modifier = Modifier, onClick: () -> Unit) {
    val online = host.online
    val statusColor = if (online) Color(0xFF00A86B) else Color(0xFFEF4444)
    val borderColor = if (online) Color(0xFF00A86B).copy(alpha = 0.15f) else Color(0xFFEF4444).copy(alpha = 0.15f)

    Card(
        onClick = onClick,
        modifier = modifier.fillMaxWidth().border(1.dp, borderColor, RoundedCornerShape(12.dp)),
        shape = RoundedCornerShape(12.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 2.dp)
    ) {
        Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min)) {
            Box(Modifier.width(4.dp).fillMaxHeight().background(statusColor))
            Column(Modifier.weight(1f).padding(14.dp)) {
                Column {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        StatusDot(statusColor, size = 10.dp)
                        Spacer(Modifier.width(8.dp))
                        Text(host.hostname, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface, maxLines = 1)
                    }
                    Spacer(Modifier.height(4.dp))
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(host.ip ?: "未知 IP", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        Spacer(Modifier.weight(1f))
                        Text(if (online) "在线" else "离线", style = MaterialTheme.typography.labelMedium, color = statusColor, fontWeight = FontWeight.SemiBold)
                        host.category?.let {
                            Spacer(Modifier.width(8.dp))
                            StatusPill(it.uppercase(), Color(0xFF4F7FFF))
                        }
                    }
                }

                Spacer(Modifier.height(14.dp))

                val latest = host.latest
                if (latest != null) {
                    Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                        MetricBar("CPU", "%.0f%%".format(latest.cpu_percent), (latest.cpu_percent / 100f).toFloat(), thresholdColor(latest.cpu_percent.toFloat()), Modifier.weight(1f))
                        MetricBar("内存", "%.0f%%".format(latest.mem_percent), (latest.mem_percent / 100f).toFloat(), thresholdColor(latest.mem_percent.toFloat()), Modifier.weight(1f))
                        MetricBar("磁盘", "%.0f%%".format(latest.disk_percent), (latest.disk_percent / 100f).toFloat(), thresholdColor(latest.disk_percent.toFloat()), Modifier.weight(1f))
                    }
                    val gpus = latest.gpus.orEmpty()
                    if (gpus.isNotEmpty()) {
                        Spacer(Modifier.height(12.dp))
                        HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant, thickness = 0.5.dp)
                        Spacer(Modifier.height(12.dp))
                        val gpuUtil = gpus.maxOf { it.util_percent }
                        val gpuMemory = gpus.maxOf { it.mem_percent }
                        val gpuTemp = gpus.maxOf { it.temp }
                        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                            MetricBar("GPU ×${gpus.size}", "%.0f%%".format(gpuUtil), (gpuUtil / 100).toFloat(), thresholdColor(gpuUtil.toFloat()), Modifier.weight(1f))
                            MetricBar("GPU 显存", "%.0f%%".format(gpuMemory), (gpuMemory / 100).toFloat(), thresholdColor(gpuMemory.toFloat()), Modifier.weight(1f))
                            MetricBar("GPU 温度", if (gpuTemp > 0) "%.0f°C".format(gpuTemp) else "—", (gpuTemp / 100).toFloat(), thresholdColor(gpuTemp.toFloat()), Modifier.weight(1f))
                        }
                        if (gpus.size == 1 && gpus.first().mem_total > 0) {
                            Text("${gpus.first().name} · ${formatBytes(gpus.first().mem_used)} / ${formatBytes(gpus.first().mem_total)}", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 9.sp, modifier = Modifier.padding(top = 5.dp))
                        }
                    }
                } else {
                    Text("等待 Agent 数据...", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp)
                }
            }
        }
    }
}

@Composable
private fun GpuOverviewCard(hosts: List<Host>, onHostClick: (String) -> Unit = {}) {
    val gpuHosts = hosts.filter { !it.latest?.gpus.isNullOrEmpty() }
    val devices = gpuHosts.flatMap { host -> host.latest?.gpus.orEmpty().map { host to it } }
    if (devices.isEmpty()) return
    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(12.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 1.dp)
    ) {
        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text("GPU 资源概览", color = MaterialTheme.colorScheme.onSurface, fontWeight = FontWeight.Bold, fontSize = 14.sp)
                    Text("${gpuHosts.size} 台加速主机 · ${devices.size} 块设备", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 11.sp, modifier = Modifier.padding(top = 2.dp))
                }
                val peak = devices.maxOfOrNull { it.second.util_percent } ?: 0.0
                StatusPill("峰值 %.0f%%".format(peak), thresholdColor(peak.toFloat()))
            }
            HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant, thickness = 0.5.dp)
            Column(
                modifier = Modifier.heightIn(max = 255.dp).verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                val sortedDevices = devices.sortedByDescending { it.second.util_percent }
                sortedDevices.forEachIndexed { index, (host, gpu) ->
                    Column(
                        verticalArrangement = Arrangement.spacedBy(6.dp),
                        modifier = Modifier.clickable { onHostClick(host.id) }
                    ) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Text(host.hostname, color = MaterialTheme.colorScheme.onSurface, fontSize = 12.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f), maxLines = 1)
                            Text(gpu.name, color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 10.sp, maxLines = 1)
                        }
                        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                            MetricBar("GPU", "%.0f%%".format(gpu.util_percent), (gpu.util_percent / 100).toFloat(), thresholdColor(gpu.util_percent.toFloat()), Modifier.weight(1f))
                            MetricBar("显存", "%.0f%%".format(gpu.mem_percent), (gpu.mem_percent / 100).toFloat(), thresholdColor(gpu.mem_percent.toFloat()), Modifier.weight(1f))
                            MetricBar("温度", if (gpu.temp > 0) "%.0f°C".format(gpu.temp) else "—", (gpu.temp / 100).toFloat(), thresholdColor(gpu.temp.toFloat()), Modifier.weight(1f))
                        }
                        if (index < sortedDevices.lastIndex) {
                            Spacer(Modifier.height(6.dp))
                            HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant, thickness = 0.5.dp)
                        }
                    }
                }
            }
        }
    }
}
