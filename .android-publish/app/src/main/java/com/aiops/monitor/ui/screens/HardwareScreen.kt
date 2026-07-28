package com.aiops.monitor.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Memory
import androidx.compose.material.icons.filled.Refresh
import androidx.navigation.NavHostController
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
import com.aiops.monitor.data.ApiClient
import com.aiops.monitor.data.SessionTicker
import com.aiops.monitor.data.models.HardwareSnapshot
import com.aiops.monitor.ui.components.*
import com.aiops.monitor.ui.viewmodel.HardwareViewModel
import kotlinx.coroutines.delay

private val HwCrit = AppRed
private val HwWarn = AppOrange
private val HwOk = AppGreen
private val HwGray = Color(0xFF8A93A3)

private fun hwColor(health: String): Color = when (health) {
    "Critical" -> HwCrit
    "Warning" -> HwWarn
    "OK" -> HwOk
    else -> HwGray
}

private fun hwHealthText(health: String): String = when (health) {
    "OK" -> "正常"; "Warning" -> "警告"; "Critical" -> "严重"; "" -> "未知"; else -> health
}

private fun hwEnum(v: String): String = when (v) {
    "Enabled" -> "已启用"; "Disabled" -> "已禁用"; "Absent" -> "未安装"
    "On" -> "已开机"; "Off" -> "已关机"
    "NotRedundant" -> "无冗余"; "N+m" -> "N+m 冗余"; "Sparing" -> "冗余备份"; "Failover" -> "故障转移"
    else -> v
}

private data class BadPart(val kind: String, val name: String, val reading: String, val status: String)

private fun isBad(h: String) = h == "Warning" || h == "Critical"

private fun badParts(sd: HardwareSnapshot): List<BadPart> {
    val out = mutableListOf<BadPart>()
    sd.temps.orEmpty().forEach {
        val over = when {
            it.upper_critical > 0 && it.reading >= it.upper_critical -> "Critical"
            it.upper_caution > 0 && it.reading >= it.upper_caution -> "Warning"
            isBad(it.status) -> it.status
            else -> ""
        }
        if (over.isNotEmpty()) out.add(BadPart("温度", it.name, "${it.reading.toInt()}°C", over))
    }
    sd.fans.orEmpty().forEach {
        val st = if (isBad(it.health)) it.health else if (isBad(it.status)) it.status else ""
        if (st.isNotEmpty()) out.add(BadPart("风扇", it.name, "${it.rpm} RPM", st))
    }
    sd.power.psus.orEmpty().forEach { if (isBad(it.health)) out.add(BadPart("电源", it.name, "${it.input_watts.toInt()}W", it.health)) }
    sd.storage.orEmpty().forEach {
        if (isBad(it.health) || it.smart_warn) {
            val nm = if (it.location.isNotBlank()) "${it.name} (${it.location})" else it.name
            out.add(BadPart("存储", nm, if (it.smart_warn) "⚠ 预测故障" else "${it.capacity_gb.toInt()}GB", if (it.smart_warn) "Critical" else it.health))
        }
    }
    sd.memory.dimms.orEmpty().forEach { if (isBad(it.health)) out.add(BadPart("内存", it.slot.ifBlank { it.name }, "${it.capacity_gb.toInt()}GB", it.health)) }
    sd.cpus.orEmpty().forEach { if (isBad(it.health)) out.add(BadPart("CPU", it.name, it.model, it.health)) }
    sd.gpus.orEmpty().forEach { if (isBad(it.health)) out.add(BadPart("GPU", it.name, it.model, it.health)) }
    sd.raid.orEmpty().forEach { if (isBad(it.health)) out.add(BadPart("RAID", it.name, it.model, it.health)) }
    sd.enclosures.orEmpty().forEach { if (isBad(it.health)) out.add(BadPart("磁盘框", it.location.ifBlank { it.name }, it.model, it.health)) }
    return out
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HardwareContent(modifier: Modifier = Modifier, refreshSignal: Int = 0) {
    val vm: HardwareViewModel = viewModel()
    val items by vm.items.collectAsState()
    val loading by vm.loading.collectAsState()
    val error by vm.error.collectAsState()

    var query by remember { mutableStateOf("") }
    var detail by remember { mutableStateOf<HardwareViewModel.HwItem?>(null) }

    LaunchedEffect(Unit) {
        vm.load()
        SessionTicker.pollWhileAlive(30_000L) { vm.load() }
    }
    LaunchedEffect(refreshSignal) { if (refreshSignal > 0) vm.load() }

    val filtered = remember(items, query) {
        val q = query.trim().lowercase()
        if (q.isEmpty()) items else items.filter {
            val sys = it.wrap.snapshot.system
            listOf(it.host.hostname, it.host.id, it.wrap.target_name, it.wrap.target_url,
                sys.manufacturer, sys.model, sys.serial_number)
                .joinToString(" ").lowercase().let { hay -> q.split(" ").all { w -> hay.contains(w) } }
        }
    }

    Column(modifier.fillMaxSize().background(MaterialTheme.colorScheme.background)) {
        OutlinedTextField(
            value = query,
            onValueChange = { query = it },
            modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
            placeholder = { Text("搜索主机名 / 型号 / 序列号") },
            singleLine = true,
            keyboardOptions = KeyboardOptions(),
            shape = RoundedCornerShape(12.dp)
        )

        when {
            loading && items.isEmpty() -> LoadingBox()
            error != null && items.isEmpty() ->
                StateBox("加载失败：$error")
            items.isEmpty() ->
                StateBox("暂无硬件数据（需在 Agent 配置 Redfish/OceanStor 目标）")
            filtered.isEmpty() ->
                StateBox("没有匹配的设备")
            else -> LazyColumn(
                Modifier.fillMaxSize(),
                contentPadding = PaddingValues(16.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                itemsIndexed(filtered, key = { i, it -> "$i#${it.host.id}#${it.wrap.target_name}" }) { _, it ->
                    HardwareCard(it) { detail = it }
                }
            }
        }
    }

    detail?.let { HardwareDetailSheet(it) { detail = null } }
}

@Composable
private fun HardwareCard(item: HardwareViewModel.HwItem, onClick: () -> Unit) {
    val sd = item.wrap.snapshot
    val sys = sd.system
    val health = item.wrap.health
    val color = hwColor(health)
    val bads = remember(sd) { badParts(sd) }
    val maxTemp = sd.temps.orEmpty().maxOfOrNull { it.reading } ?: 0.0

    Card(
        Modifier.fillMaxWidth().clickable(onClick = onClick),
        shape = RoundedCornerShape(14.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 1.dp)
    ) {
        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                Box(
                    Modifier.size(30.dp).clip(CircleShape).background(color.copy(alpha = 0.15f)),
                    contentAlignment = Alignment.Center
                ) {
                    Text(
                        when (health) { "OK" -> "✓"; "Warning" -> "⚠"; "Critical" -> "✕"; else -> "?" },
                        color = color, fontWeight = FontWeight.Bold, fontSize = 14.sp
                    )
                }
                Column(Modifier.weight(1f)) {
                    Text(item.wrap.target_name.ifBlank { item.wrap.target_url }, fontWeight = FontWeight.Bold,
                        maxLines = 1, overflow = TextOverflow.Ellipsis, color = MaterialTheme.colorScheme.onSurface)
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                        StatusDot(if (item.online) HwOk else HwGray, size = 6.dp)
                        Text("${item.host.hostname} · ${hwHealthText(health)}", fontSize = 12.sp,
                            color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    }
                    val model = listOf(sys.manufacturer, sys.model).filter { it.isNotBlank() }.joinToString(" ")
                    if (model.isNotBlank()) Text(model, fontSize = 11.sp, color = HwGray, maxLines = 1, overflow = TextOverflow.Ellipsis)
                }
                if (bads.isNotEmpty()) StatusPill("${bads.size} 项异常", HwCrit)
            }
            // 快速统计 chips
            FlowChips(buildList {
                if (sd.cpus.orEmpty().isNotEmpty()) add("CPU" to "${sd.cpus.orEmpty().size}×${sd.cpus.orEmpty().firstOrNull()?.cores ?: 0}C")
                if (sd.memory.total_gb > 0) add("内存" to "${sd.memory.total_gb.toInt()}GB")
                if (maxTemp > 0) add("最高温" to "${maxTemp.toInt()}°C")
                if (sd.power.total_watts > 0) add("功耗" to "${sd.power.total_watts.toInt()}W")
                if (sd.storage.orEmpty().isNotEmpty()) add("盘" to "${sd.storage.orEmpty().size}")
                if (sd.fans.orEmpty().isNotEmpty()) add("风扇" to "${sd.fans.orEmpty().size}")
                if (sd.gpus.orEmpty().isNotEmpty()) add("GPU" to "${sd.gpus.orEmpty().size}")
                if (sd.enclosures.orEmpty().isNotEmpty()) add("磁盘框" to "${sd.enclosures.orEmpty().size}")
            })
            Text("点击查看详情 →", fontSize = 11.sp, color = MaterialTheme.colorScheme.primary)
        }
    }
}

@Composable
private fun FlowChips(chips: List<Pair<String, String>>) {
    if (chips.isEmpty()) return
    // 简易两行流式布局
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        chips.chunked(4).forEach { row ->
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                row.forEach { (k, v) ->
                    Box(
                        Modifier.clip(RoundedCornerShape(6.dp)).background(MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.5f))
                            .padding(horizontal = 8.dp, vertical = 4.dp)
                    ) {
                        Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                            Text(k, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            Text(v, fontSize = 11.sp, fontWeight = FontWeight.Medium, color = MaterialTheme.colorScheme.onSurface)
                        }
                    }
                }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun HardwareDetailSheet(item: HardwareViewModel.HwItem, onDismiss: () -> Unit) {
    val sd = item.wrap.snapshot
    val sys = sd.system
    val bads = remember(sd) { badParts(sd) }
    val maxTemp = sd.temps.orEmpty().maxOfOrNull { it.reading } ?: 0.0
    var showAi by remember { mutableStateOf(false) }

    ModalBottomSheet(onDismissRequest = onDismiss, containerColor = MaterialTheme.colorScheme.surface) {
        Column(
            Modifier.fillMaxWidth().padding(horizontal = 16.dp).padding(bottom = 24.dp)
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
                Text(
                    listOf(item.wrap.target_name, item.host.hostname, sys.model).filter { it.isNotBlank() }.joinToString(" · "),
                    style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold,
                    modifier = Modifier.weight(1f)
                )
                TextButton(onClick = { showAi = true }) { Text("AI 诊断") }
            }

            if (sd.error.isNotBlank()) {
                Surface(color = HwCrit.copy(alpha = 0.1f), shape = RoundedCornerShape(8.dp)) {
                    Text("采集错误：${sd.error}", Modifier.padding(10.dp), color = HwCrit, fontSize = 12.sp)
                }
            }

            // KPI 行
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                KpiBox("整机健康", hwHealthText(item.wrap.health), hwColor(item.wrap.health), Modifier.weight(1f))
                KpiBox("异常部件", "${bads.size}", if (bads.isEmpty()) HwOk else HwCrit, Modifier.weight(1f))
                KpiBox("最高温", if (maxTemp > 0) "${maxTemp.toInt()}°C" else "-", MaterialTheme.colorScheme.onSurface, Modifier.weight(1f))
                KpiBox("功耗", if (sd.power.total_watts > 0) "${sd.power.total_watts.toInt()}W" else "-", MaterialTheme.colorScheme.onSurface, Modifier.weight(1f))
            }

            // 设备信息
            SectionCard(title = "设备信息") {
                infoIf("厂商", sys.manufacturer)
                infoIf("型号", sys.model)
                infoIf("序列号", sys.serial_number.ifBlank { sys.sku })
                infoIf("资产编号", sys.asset_tag)
                infoIf("BIOS", sys.bios_version)
                infoIf("BMC", listOf(sys.bmc_model, sys.bmc_firmware).filter { it.isNotBlank() }.joinToString(" "))
                infoIf("电源状态", hwEnum(sys.power_state))
                infoIf("电源冗余", hwEnum(sd.power.redundancy))
                InfoRow("BMC 地址", item.wrap.target_url)
            }

            // 需要关注
            if (bads.isNotEmpty()) {
                SectionCard(title = "需要关注（${bads.size}）") {
                    bads.forEach { b ->
                        Row(Modifier.fillMaxWidth().padding(vertical = 4.dp), Arrangement.SpaceBetween, Alignment.CenterVertically) {
                            Text("${b.kind} · ${b.name}", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurface,
                                modifier = Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis)
                            Text(b.reading, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            Spacer(Modifier.width(8.dp))
                            StatusPill(hwHealthText(b.status), hwColor(b.status))
                        }
                    }
                }
            }

            // BMC 事件（含触发部件）
            if (sd.events.orEmpty().isNotEmpty()) {
                SectionCard(title = "事件日志（${sd.events.orEmpty().size}）") {
                    sd.events.orEmpty().take(20).forEach { e ->
                        Column(Modifier.fillMaxWidth().padding(vertical = 5.dp)) {
                            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                                StatusPill(hwHealthText(e.severity), hwColor(e.severity))
                                if (e.component.isNotBlank()) Text(e.component, fontSize = 11.sp, fontWeight = FontWeight.Medium, color = MaterialTheme.colorScheme.primary)
                                Text(fmtTime(e.created), fontSize = 10.sp, color = HwGray)
                            }
                            Text(e.message, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurface)
                        }
                        HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant.copy(alpha = 0.3f))
                    }
                }
            }

            // 部件明细
            if (sd.cpus.orEmpty().isNotEmpty()) SectionCard(title = "CPU（${sd.cpus.orEmpty().size}）") {
                sd.cpus.orEmpty().forEach { InfoRow("${it.name}  ${it.cores}C/${it.threads}T", "${it.model.take(28)}  ${hwHealthText(it.health)}") }
            }
            if (sd.gpus.orEmpty().isNotEmpty()) SectionCard(title = "GPU / 加速卡（${sd.gpus.orEmpty().size}）") {
                sd.gpus.orEmpty().forEach { InfoRow(it.name, "${it.model}  ${hwHealthText(it.health)}") }
            }
            if (sd.memory.dimms.orEmpty().isNotEmpty()) SectionCard(title = "内存（${sd.memory.total_gb.toInt()}GB）") {
                sd.memory.dimms.orEmpty().forEach { InfoRow("${it.slot.ifBlank { it.name }}  ${it.capacity_gb.toInt()}GB ${it.type}", "${it.speed_mhz}MHz  ${hwHealthText(it.health)}") }
            }
            if (sd.storage.orEmpty().isNotEmpty()) SectionCard(title = "存储（${sd.storage.orEmpty().size}）") {
                sd.storage.orEmpty().forEach {
                    val life = if (it.life_left_pct >= 0) " · 寿命${it.life_left_pct.toInt()}%" else ""
                    InfoRow("${it.location.ifBlank { it.name }}  ${it.capacity_gb.toInt()}GB", "${it.media_type}${if (it.smart_warn) " · ⚠预测故障" else ""}$life  ${hwHealthText(it.health)}")
                }
            }
            if (sd.raid.orEmpty().isNotEmpty()) SectionCard(title = "RAID / 存储控制器（${sd.raid.orEmpty().size}）") {
                sd.raid.orEmpty().forEach { InfoRow(it.name, "${it.model}  ${hwHealthText(it.health)}") }
            }
            if (sd.enclosures.orEmpty().isNotEmpty()) SectionCard(title = "磁盘框（${sd.enclosures.orEmpty().size}）") {
                sd.enclosures.orEmpty().forEach { InfoRow("${it.location.ifBlank { it.name }}${if (it.temperature_c > 0) "  ${it.temperature_c.toInt()}°C" else ""}", hwHealthText(it.health)) }
            }
            if (sd.power.psus.orEmpty().isNotEmpty()) SectionCard(title = "电源（${sd.power.psus.orEmpty().size}）") {
                sd.power.psus.orEmpty().forEach { InfoRow("${it.name}  ${it.input_watts.toInt()}W", "${it.model.take(20)}  ${hwHealthText(it.health)}") }
            }
            if (sd.fans.orEmpty().isNotEmpty()) SectionCard(title = "风扇（${sd.fans.orEmpty().size}）") {
                sd.fans.orEmpty().forEach { InfoRow(it.name, "${it.rpm} RPM  ${hwHealthText(it.health)}") }
            }
            if (sd.temps.orEmpty().isNotEmpty()) SectionCard(title = "温度传感器（${sd.temps.orEmpty().size}）") {
                sd.temps.orEmpty().forEach {
                    val over = it.upper_critical > 0 && it.reading >= it.upper_critical
                    InfoRow(it.name, "${it.reading.toInt()}°C${if (it.upper_critical > 0) " / 阈值${it.upper_critical.toInt()}°C" else ""}${if (over) " ⚠" else ""}")
                }
            }
            if (sd.firmware.orEmpty().isNotEmpty()) SectionCard(title = "固件（${sd.firmware.orEmpty().size}）") {
                sd.firmware.orEmpty().forEach { InfoRow(it.name, it.version) }
            }
        }
    }
    if (showAi) {
        val ctx = buildString {
            appendLine("主机：${item.host.hostname} BMC：${item.wrap.target_url}")
            appendLine("整机健康：${hwHealthText(item.wrap.health)}")
            appendLine("型号：${sys.manufacturer} ${sys.model} SN=${sys.serial_number}")
            appendLine("异常部件：${bads.size} 最高温：${maxTemp.toInt()}°C 功耗：${sd.power.total_watts.toInt()}W")
            bads.take(20).forEach { appendLine("- ${it.kind} ${it.name} ${it.reading} ${it.status}") }
            sd.events.orEmpty().take(10).forEach { appendLine("事件[${it.severity}] ${it.message}") }
        }
        AiAssistSheet(
            title = "AI 硬件诊断 · ${item.wrap.target_name.ifBlank { item.host.hostname }}",
            task = "hardware_diagnosis",
            context = ctx.take(14000),
            onDismiss = { showAi = false }
        )
    }
}

@Composable
private fun KpiBox(label: String, value: String, color: Color, modifier: Modifier = Modifier) {
    Surface(modifier, color = MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.4f), shape = RoundedCornerShape(10.dp)) {
        Column(Modifier.padding(vertical = 10.dp), horizontalAlignment = Alignment.CenterHorizontally) {
            Text(value, fontWeight = FontWeight.Bold, color = color, fontSize = 15.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Text(label, fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

@Composable
private fun ColumnScope.infoIf(label: String, value: String) {
    if (value.isNotBlank()) InfoRow(label, value)
}

private fun fmtTime(v: String): String {
    if (v.isBlank()) return "-"
    // RFC3339 → 取 "MM-dd HH:mm"
    return try {
        val t = v.replace('T', ' ')
        if (t.length >= 16) t.substring(5, 16) else t
    } catch (_: Exception) { v }
}
