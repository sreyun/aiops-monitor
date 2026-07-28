package com.aiops.monitor.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.FileDownload
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavHostController
import com.aiops.monitor.data.ApiClient
import com.aiops.monitor.data.SessionTicker
import com.aiops.monitor.data.models.HyperVGuest
import com.aiops.monitor.data.models.HyperVInventory
import com.aiops.monitor.ui.HyperVExport
import com.aiops.monitor.ui.Routes
import com.aiops.monitor.ui.components.*
import com.aiops.monitor.ui.viewmodel.HyperVViewModel
import kotlinx.coroutines.delay

private val HvGreen = Color(0xFF00A86B)
private val HvGray = Color(0xFF8A93A3)
private val HvRed = Color(0xFFEF4444)
private val HvOrange = Color(0xFFF59E0B)
private val HvBlue = Color(0xFF4F7FFF)

private fun hvStateText(s: String): String = when (s) {
    "Running" -> "运行中"; "Off" -> "已关机"; "Paused" -> "已暂停"; "Saved" -> "已保存"
    "Starting" -> "启动中"; "Stopping" -> "停止中"; "" -> "未知"; else -> s
}

private fun hvAbnormal(g: HyperVGuest): Boolean {
    val h = g.health.lowercase(); val r = g.repl_health.lowercase()
    return h == "warning" || h == "critical" || r == "warning" || r == "critical"
}

private fun hvStateColor(g: HyperVGuest): Color = when {
    hvAbnormal(g) -> if (g.health.equals("critical", true)) HvRed else HvOrange
    g.state == "Running" -> HvGreen
    g.state == "Paused" -> HvOrange
    else -> HvGray
}

private fun hvUptime(sec: Long): String {
    if (sec <= 0) return "—"
    val d = sec / 86400; val h = (sec % 86400) / 3600; val m = (sec % 3600) / 60
    return when { d > 0 -> "${d}天${h}时"; h > 0 -> "${h}时${m}分"; else -> "${m}分" }
}

private fun hvGB(gb: Double): String = if (gb <= 0) "—" else if (gb < 1) "%.0f MB".format(gb * 1024) else "%.1f GB".format(gb)

private fun hvMatches(g: HyperVGuest, q: String): Boolean {
    if (q.isBlank()) return true
    val hay = (listOf(g.name, g.state, g.linked_host_name ?: "") + g.ip_addresses.orEmpty()).joinToString(" ").lowercase()
    return q.trim().lowercase().split(" ").all { hay.contains(it) }
}

private enum class HvFilter(val label: String) { All("全部"), Running("运行中"), NotRunning("非运行") }

/**
 * Hyper-V 虚拟机内容（基础设施中枢「虚拟机」子标签）。无 Scaffold，供 InfraHubScreen 内嵌。
 * 按物理宿主机分组展示 guest；点 VM 打开详情（概览/CPU/内存/磁盘/网卡/检查点）。
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HyperVContent(navController: NavHostController, modifier: Modifier = Modifier, refreshSignal: Int = 0) {
    val vm: HyperVViewModel = viewModel()
    val inventories by vm.inventories.collectAsState()
    val loading by vm.loading.collectAsState()
    val error by vm.error.collectAsState()

    var query by remember { mutableStateOf("") }
    var filter by remember { mutableStateOf(HvFilter.Running) } // 默认「运行中」，与 Web 一致
    var detail by remember { mutableStateOf<HyperVGuest?>(null) }
    var exportMenu by remember { mutableStateOf(false) }
    val context = LocalContext.current

    LaunchedEffect(Unit) {
        vm.load()
        SessionTicker.pollWhileAlive(15_000L) { vm.load() }
    }
    LaunchedEffect(refreshSignal) { if (refreshSignal > 0) vm.load() }

    fun visibleGuests(inv: HyperVInventory): List<HyperVGuest> = inv.guests.orEmpty().filter { g ->
        hvMatches(g, query) && when (filter) {
            HvFilter.All -> true
            HvFilter.Running -> g.state == "Running"
            HvFilter.NotRunning -> g.state != "Running"
        }
    }

    val totalVMs = inventories.sumOf { it.guests.orEmpty().size }
    val totalBad = inventories.sumOf { it.guests.orEmpty().count { g -> hvAbnormal(g) } }

    Column(modifier.fillMaxSize().background(MaterialTheme.colorScheme.background)) {
        // 工具栏：搜索 + 状态筛选 + 导出
        Column(Modifier.padding(horizontal = 16.dp, vertical = 6.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedTextField(
                value = query,
                onValueChange = { query = it },
                modifier = Modifier.fillMaxWidth(),
                placeholder = { Text("搜索 VM 名称 / IP / 状态") },
                leadingIcon = { Icon(Icons.Default.Search, null, tint = MaterialTheme.colorScheme.onSurfaceVariant) },
                singleLine = true,
                shape = RoundedCornerShape(12.dp)
            )
            Row(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
                HvFilter.entries.forEach { f ->
                    FilterChip(selected = filter == f, onClick = { filter = f }, label = { Text(f.label) }, colors = chipColors())
                }
                Spacer(Modifier.weight(1f))
                Box {
                    FilterChip(
                        selected = false,
                        onClick = { exportMenu = true },
                        label = { Text("导出") },
                        leadingIcon = { Icon(Icons.Default.FileDownload, null, Modifier.size(16.dp)) },
                        colors = chipColors()
                    )
                    DropdownMenu(expanded = exportMenu, onDismissRequest = { exportMenu = false }) {
                        DropdownMenuItem(
                            text = { Text("Excel（CSV）") },
                            onClick = {
                                exportMenu = false
                                HyperVExport.shareExcel(context, inventories)
                            }
                        )
                        DropdownMenuItem(
                            text = { Text("PDF（打印）") },
                            onClick = {
                                exportMenu = false
                                HyperVExport.printPdf(context, inventories)
                            }
                        )
                    }
                }
            }
            if (totalVMs > 0) {
                Text(
                    "共 $totalVMs 台虚拟机" + (if (totalBad > 0) " · $totalBad 台需关注" else ""),
                    style = MaterialTheme.typography.labelSmall,
                    color = if (totalBad > 0) HvOrange else MaterialTheme.colorScheme.onSurfaceVariant
                )
            }
        }

        when {
            loading && inventories.isEmpty() -> LoadingBox(Modifier.fillMaxSize())
            error != null && inventories.isEmpty() -> StateBox("加载失败：$error", Modifier.fillMaxSize(), onRetry = vm::load)
            inventories.isEmpty() -> StateBox("暂无 Hyper-V 宿主机数据\n请确认物理机已安装并更新 Agent（含 Hyper-V 采集），且已启用 Hyper-V 角色", Modifier.fillMaxSize())
            else -> {
                val hostsWithVisible = inventories.map { it to visibleGuests(it) }.filter { it.second.isNotEmpty() }
                if (hostsWithVisible.isEmpty()) {
                    StateBox("没有匹配的虚拟机", Modifier.fillMaxSize())
                } else {
                    LazyColumn(
                        Modifier.fillMaxSize(),
                        contentPadding = PaddingValues(16.dp),
                        verticalArrangement = Arrangement.spacedBy(14.dp)
                    ) {
                        hostsWithVisible.forEach { (inv, guests) ->
                            item(key = "host-${inv.host_id}") { HostGroupHeader(inv) }
                            itemsIndexed(guests, key = { i, g -> "${inv.host_id}#$i#${g.id.ifBlank { g.name }}" }) { _, g ->
                                VmCard(g) { detail = g }
                            }
                        }
                    }
                }
            }
        }
    }

    detail?.let { g -> VmDetailSheet(g, onDismiss = { detail = null }, onOpenHost = { hid -> detail = null; navController.navigate(Routes.hostDetail(hid)) }) }
}

@Composable
private fun HostGroupHeader(inv: HyperVInventory) {
    val guests = inv.guests.orEmpty()
    val running = guests.count { it.state == "Running" }
    val bad = guests.count { hvAbnormal(it) }
    Row(Modifier.fillMaxWidth().padding(top = 4.dp), verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f)) {
            Text(inv.host_name.ifBlank { inv.host_id }, fontWeight = FontWeight.Bold, fontSize = 15.sp, color = MaterialTheme.colorScheme.onSurface, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Text("${guests.size} 虚拟机 · $running 运行中" + (if (bad > 0) " · $bad 需关注" else ""), fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        if (bad > 0) StatusPill("$bad 需关注", HvOrange)
    }
}

@Composable
private fun VmCard(g: HyperVGuest, onClick: () -> Unit) {
    val running = g.state == "Running"
    val color = hvStateColor(g)
    Card(
        Modifier.fillMaxWidth().clickable(onClick = onClick),
        shape = RoundedCornerShape(14.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 1.dp)
    ) {
        Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min)) {
            Box(Modifier.width(4.dp).fillMaxHeight().background(color))
            Column(Modifier.weight(1f).padding(14.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    StatusDot(color, size = 9.dp)
                    Text(g.name, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
                    StatusPill(hvStateText(g.state), color)
                }
                Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    VmStat("CPU", if (running) "%.0f%%".format(g.cpu_usage) else "—", Modifier.weight(1f))
                    VmStat("内存", if (running && g.mem_assigned_mb > 0) "${g.mem_assigned_mb.toInt()}MB" else "—", Modifier.weight(1f))
                    VmStat("vCPU", if (g.processor_count > 0) "${g.processor_count}" else "—", Modifier.weight(1f))
                    VmStat("运行", if (running) hvUptime(g.uptime_sec) else "—", Modifier.weight(1f))
                }
                val linked = g.linked_host_name
                if (!linked.isNullOrBlank()) {
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                        StatusPill("纳管主机", HvBlue)
                        Text(linked, fontSize = 11.sp, color = HvBlue, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    }
                }
            }
        }
    }
}

@Composable
private fun VmStat(label: String, value: String, modifier: Modifier = Modifier) {
    Column(modifier) {
        Text(value, fontWeight = FontWeight.Bold, fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurface, maxLines = 1)
        Text(label, fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun VmDetailSheet(g: HyperVGuest, onDismiss: () -> Unit, onOpenHost: (String) -> Unit) {
    val running = g.state == "Running"
    ModalBottomSheet(onDismissRequest = onDismiss, containerColor = MaterialTheme.colorScheme.surface) {
        Column(
            Modifier.fillMaxWidth().padding(horizontal = 16.dp).padding(bottom = 24.dp).verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                StatusDot(hvStateColor(g), size = 10.dp)
                Text(g.name, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis)
                StatusPill(hvStateText(g.state), hvStateColor(g))
            }

            // 概览
            SectionCard(title = "概览") {
                infoIf("状态", hvStateText(g.state))
                InfoRow("运行时长", if (running) hvUptime(g.uptime_sec) else "—")
                infoIf("健康", when (g.health.lowercase()) { "ok" -> "正常"; "warning" -> "警告"; "critical" -> "严重"; else -> g.health })
                infoIf("集成服务", g.integration_state)
                if (g.generation > 0) InfoRow("世代", "第 ${g.generation} 代")
                infoIf("配置版本", g.version)
                if (g.repl_state.isNotBlank() && g.repl_state != "Disabled") {
                    InfoRow("副本状态", "${g.repl_state} · ${g.repl_health}")
                }
                val linkedId = g.linked_host_id
                val linked = g.linked_host_name
                if (!linked.isNullOrBlank() && !linkedId.isNullOrBlank()) {
                    Row(
                        Modifier.fillMaxWidth().clip(RoundedCornerShape(8.dp)).clickable { onOpenHost(linkedId) }.padding(vertical = 6.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text("关联纳管主机", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.weight(1f))
                        Text("$linked ↗", fontSize = 12.sp, color = HvBlue, fontWeight = FontWeight.Medium)
                    }
                }
            }

            // CPU
            SectionCard(title = "CPU") {
                MetricBar("占宿主 CPU", if (running) "%.0f%%".format(g.cpu_usage) else "—", if (running) (g.cpu_usage / 100).toFloat() else 0f, thresholdColor(g.cpu_usage.toFloat()))
                InfoRow("vCPU 数", if (g.processor_count > 0) "${g.processor_count}" else "—")
            }

            // 内存
            SectionCard(title = "内存") {
                val asg = g.mem_assigned_mb; val dem = g.mem_demand_mb
                if (running && g.dynamic_mem_enabled && asg > 0) {
                    val pct = (dem / asg).toFloat()
                    MetricBar("内存压力(需求/分配)", "${dem.toInt()} / ${asg.toInt()} MB", pct, thresholdColor(pct * 100))
                }
                InfoRow("已分配", if (running && asg > 0) "${asg.toInt()} MB" else "—")
                InfoRow("需求", if (running && dem > 0) "${dem.toInt()} MB" else "—")
                if (g.mem_startup_mb > 0) InfoRow("启动内存", "${g.mem_startup_mb.toInt()} MB")
                if (g.dynamic_mem_enabled) {
                    InfoRow("动态范围", "${g.mem_min_mb.toInt()} ~ ${g.mem_max_mb.toInt()} MB")
                } else {
                    InfoRow("内存类型", "静态")
                }
            }

            // 硬盘
            val disks = g.disks.orEmpty()
            SectionCard(title = "硬盘 · ${if (disks.isNotEmpty()) disks.size else g.vhd_count}") {
                if (disks.isEmpty()) {
                    Text("无磁盘明细（需更新 Agent 采集）", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                } else {
                    disks.forEach { d ->
                        val ctrl = (d.controller_type + " ${d.controller_number}:${d.controller_location}").trim()
                        InfoRow(d.path.substringAfterLast('\\').ifBlank { d.path }, "$ctrl · ${hvGB(d.file_size_gb)}")
                    }
                }
            }

            // 网络
            val nics = g.nics.orEmpty()
            val ips = g.ip_addresses.orEmpty()
            SectionCard(title = "网络 · ${if (nics.isNotEmpty()) nics.size else if (ips.isNotEmpty()) 1 else 0}") {
                if (nics.isEmpty()) {
                    if (ips.isNotEmpty()) InfoRow("IP", ips.joinToString(", "))
                    else Text("无网卡明细（需更新 Agent 采集）", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                } else {
                    nics.forEach { n ->
                        val sub = listOf(
                            n.switch.ifBlank { "—" },
                            n.status.ifBlank { if (n.connected) "已连接" else "—" },
                            n.ip_addresses.orEmpty().joinToString(", ").ifBlank { "—" }
                        ).joinToString(" · ")
                        InfoRow(n.name.ifBlank { "网卡" }, sub)
                    }
                }
            }

            // 检查点
            val cps = g.checkpoints.orEmpty()
            if (cps.isNotEmpty()) {
                SectionCard(title = "检查点 · ${cps.size}") {
                    cps.forEach { c -> InfoRow(c.name, c.created.replace("T", " ").take(19)) }
                }
            }
        }
    }
}

@Composable
private fun ColumnScope.infoIf(label: String, value: String) {
    if (value.isNotBlank()) InfoRow(label, value)
}
