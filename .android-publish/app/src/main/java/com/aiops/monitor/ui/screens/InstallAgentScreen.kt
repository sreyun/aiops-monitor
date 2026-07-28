package com.aiops.monitor.ui.screens

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Visibility
import androidx.compose.material.icons.filled.VisibilityOff
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.navigation.NavHostController
import com.aiops.monitor.data.ApiClient
import com.aiops.monitor.data.models.InstallInfo
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.net.URLEncoder

private enum class InstallOs { LINUX, WINDOWS, MACOS }
private enum class InstallMode { NORMAL, RELAY, MULTI }

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun InstallAgentScreen(navController: NavHostController, modifier: Modifier = Modifier) {
    val scope = rememberCoroutineScope()
    val context = LocalContext.current
    var info by remember { mutableStateOf<InstallInfo?>(null) }
    var loading by remember { mutableStateOf(true) }
    var error by remember { mutableStateOf<String?>(null) }
    var tokenRevealed by remember { mutableStateOf(false) }
    var category by remember { mutableStateOf("") }
    var logPaths by remember { mutableStateOf("") }
    var showLogPaths by remember { mutableStateOf(false) }
    var os by remember { mutableStateOf(InstallOs.LINUX) }
    var mode by remember { mutableStateOf(InstallMode.NORMAL) }
    var relayGatewayIp by remember { mutableStateOf("") }
    var multiServerList by remember { mutableStateOf("") }
    var snack by remember { mutableStateOf<String?>(null) }

    fun reload() {
        scope.launch {
            loading = true
            error = null
            try {
                info = withContext(Dispatchers.IO) { ApiClient.api.installInfo() }
            } catch (e: Exception) {
                error = e.message ?: "读取安装信息失败"
            } finally {
                loading = false
            }
        }
    }

    LaunchedEffect(Unit) { reload() }

    val server = info?.server_url?.trimEnd('/') ?: ApiClient.serverHost.trimEnd('/')
    val token = info?.token.orEmpty()
    val cmds = remember(info, category, logPaths, os, mode, relayGatewayIp, multiServerList) {
        buildInstallCommands(server, token, category, logPaths, os, mode, relayGatewayIp, multiServerList)
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("安装 Agent", fontWeight = FontWeight.Bold, style = MaterialTheme.typography.titleMedium) },
                navigationIcon = {
                    IconButton(onClick = { navController.popBackStack() }) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "返回")
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background)
            )
        },
        snackbarHost = {
            if (snack != null) {
                Snackbar(
                    modifier = Modifier.padding(16.dp),
                    action = { TextButton(onClick = { snack = null }) { Text("关闭") } }
                ) { Text(snack.orEmpty()) }
            }
        },
        containerColor = MaterialTheme.colorScheme.background
    ) { padding ->
        if (loading && info == null) {
            Box(Modifier.fillMaxSize().padding(padding), contentAlignment = Alignment.Center) {
                CircularProgressIndicator()
            }
            return@Scaffold
        }
        Column(
            modifier
                .fillMaxSize()
                .padding(padding)
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(14.dp)
        ) {
            error?.let { Text(it, color = Color(0xFFEF4444), style = MaterialTheme.typography.bodySmall) }

            // Token
            Card(
                shape = RoundedCornerShape(16.dp),
                colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface)
            ) {
                Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    Text("安装 Token", fontWeight = FontWeight.Bold, color = Color(0xFF4F7FFF))
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(
                            maskToken(token, tokenRevealed),
                            modifier = Modifier.weight(1f),
                            fontFamily = FontFamily.Monospace,
                            fontSize = 13.sp,
                        )
                        IconButton(onClick = { tokenRevealed = !tokenRevealed }) {
                            Icon(
                                if (tokenRevealed) Icons.Default.VisibilityOff else Icons.Default.Visibility,
                                contentDescription = null,
                                tint = MaterialTheme.colorScheme.onSurfaceVariant
                            )
                        }
                        TextButton(onClick = {
                            scope.launch {
                                try {
                                    val r = withContext(Dispatchers.IO) { ApiClient.api.resetInstallToken() }
                                    info = info?.copy(token = r.token) ?: InstallInfo(server, r.token, info?.require_token == true)
                                    snack = "Token 已重置"
                                } catch (e: Exception) {
                                    snack = e.message ?: "重置失败"
                                }
                            }
                        }) { Text("重置", fontSize = 12.sp) }
                    }
                    if (info?.require_token == true) {
                        Text("当前服务端要求安装时携带 Token", style = MaterialTheme.typography.labelSmall, color = Color(0xFFF59E0B))
                    }
                }
            }

            OutlinedTextField(
                value = category,
                onValueChange = { category = it },
                modifier = Modifier.fillMaxWidth(),
                label = { Text("分类（可选）") },
                placeholder = { Text("如：生产 / 机房A") },
                singleLine = true,
                shape = RoundedCornerShape(12.dp),
            )

            TextButton(onClick = { showLogPaths = !showLogPaths }) {
                Text(if (showLogPaths) "收起日志路径" else "展开日志路径（可选）", fontSize = 13.sp)
            }
            if (showLogPaths) {
                OutlinedTextField(
                    value = logPaths,
                    onValueChange = { logPaths = it },
                    modifier = Modifier.fillMaxWidth(),
                    label = { Text("日志路径") },
                    placeholder = { Text("每行一个路径，或逗号分隔") },
                    minLines = 2,
                    shape = RoundedCornerShape(12.dp),
                )
            }

            // OS tabs
            Text("操作系统", fontWeight = FontWeight.Bold, color = Color(0xFF4F7FFF))
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                InstallOs.entries.forEach { item ->
                    FilterChip(
                        selected = os == item,
                        onClick = { os = item },
                        label = {
                            Text(
                                when (item) {
                                    InstallOs.LINUX -> "Linux"
                                    InstallOs.WINDOWS -> "Windows"
                                    InstallOs.MACOS -> "macOS"
                                }
                            )
                        }
                    )
                }
            }

            // Mode
            Text("安装模式", fontWeight = FontWeight.Bold, color = Color(0xFF4F7FFF))
            Row(
                Modifier.horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                InstallMode.entries.forEach { item ->
                    FilterChip(
                        selected = mode == item,
                        onClick = { mode = item },
                        label = {
                            Text(
                                when (item) {
                                    InstallMode.NORMAL -> "标准"
                                    InstallMode.RELAY -> "网关中继"
                                    InstallMode.MULTI -> "多服务端"
                                }
                            )
                        }
                    )
                }
            }

            if (mode == InstallMode.RELAY) {
                OutlinedTextField(
                    value = relayGatewayIp,
                    onValueChange = { relayGatewayIp = it },
                    modifier = Modifier.fillMaxWidth(),
                    label = { Text("网关内网 IP") },
                    placeholder = { Text("如 192.168.1.10") },
                    singleLine = true,
                    shape = RoundedCornerShape(12.dp),
                )
            }
            if (mode == InstallMode.MULTI) {
                OutlinedTextField(
                    value = multiServerList,
                    onValueChange = { multiServerList = it },
                    modifier = Modifier.fillMaxWidth(),
                    label = { Text("多服务端列表") },
                    placeholder = { Text("每行：服务器地址 [token]") },
                    minLines = 3,
                    shape = RoundedCornerShape(12.dp),
                )
            }

            cmds.forEach { block ->
                CommandBlock(
                    title = block.title,
                    hint = block.hint,
                    command = block.command,
                    onCopy = {
                        copyText(context, block.command)
                        snack = "已复制到剪贴板"
                    }
                )
            }
        }
    }
}

@Composable
private fun CommandBlock(title: String, hint: String, command: String, onCopy: () -> Unit) {
    Card(
        shape = RoundedCornerShape(16.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface)
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(title, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
                IconButton(onClick = onCopy) {
                    Icon(Icons.Default.ContentCopy, contentDescription = "复制", tint = Color(0xFF4F7FFF))
                }
            }
            if (hint.isNotBlank()) {
                Text(hint, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            Text(
                command,
                fontFamily = FontFamily.Monospace,
                fontSize = 12.sp,
                color = MaterialTheme.colorScheme.onSurface,
                modifier = Modifier
                    .fillMaxWidth()
                    .horizontalScroll(rememberScrollState())
            )
            OutlinedButton(onClick = onCopy, modifier = Modifier.fillMaxWidth(), shape = RoundedCornerShape(10.dp)) {
                Icon(Icons.Default.ContentCopy, null, Modifier.size(16.dp))
                Spacer(Modifier.width(6.dp))
                Text("复制命令", fontSize = 13.sp)
            }
        }
    }
}

private data class CmdBlock(val title: String, val hint: String, val command: String)

private const val PS_TLS12 =
    "[Net.ServicePointManager]::SecurityProtocol=[Net.ServicePointManager]::SecurityProtocol -bor 3072; "

private fun maskToken(t: String, revealed: Boolean): String {
    if (t.isBlank()) return "（无 Token）"
    if (revealed) return t
    if (t.length <= 8) return "••••••••"
    return t.take(4) + "••••••••" + t.takeLast(4)
}

private fun enc(s: String): String = URLEncoder.encode(s, "UTF-8")

private fun buildQuery(token: String, category: String, logPaths: String, serversJson: String?): String {
    var q = "token=${enc(token)}"
    if (category.isNotBlank()) q += "&category=${enc(category.trim())}"
    if (logPaths.isNotBlank()) q += "&log_paths=${enc(logPaths.trim())}"
    if (!serversJson.isNullOrBlank()) q += "&servers_json=${enc(serversJson)}"
    return q
}

private fun parseMultiServers(text: String): String? {
    val servers = text.lines().map { it.trim() }.filter { it.isNotBlank() }.mapNotNull { line ->
        val parts = line.split(Regex("\\s+"), limit = 2)
        val server = parts.getOrNull(0) ?: return@mapNotNull null
        val tok = parts.getOrNull(1).orEmpty()
        """{"server":"${server.replace("\"", "\\\"")}","token":"${tok.replace("\"", "\\\"")}"}"""
    }
    return if (servers.isEmpty()) null else "[" + servers.joinToString(",") + "]"
}

private fun buildInstallCommands(
    server: String,
    token: String,
    category: String,
    logPaths: String,
    os: InstallOs,
    mode: InstallMode,
    gatewayIp: String,
    multiText: String,
): List<CmdBlock> {
    val out = mutableListOf<CmdBlock>()
    when (mode) {
        InstallMode.RELAY -> {
            val q = buildQuery(token, category, logPaths, null)
            val gwIP = gatewayIp.trim().ifBlank { "<网关IP>" }
            val relay = "http://$gwIP:8529"
            val (gwCmd, inCmd) = when (os) {
                InstallOs.WINDOWS ->
                    "${PS_TLS12}irm \"$server/install-relay.ps1?$q\" | iex" to
                        "${PS_TLS12}irm \"$relay/install.ps1?$q\" | iex"
                InstallOs.MACOS ->
                    "curl -fsSL \"$server/install-relay.sh?$q\" | sh" to
                        "curl -fsSL \"$relay/install.sh?$q\" | sh"
                InstallOs.LINUX ->
                    "curl -fsSL \"$server/install-relay.sh?$q\" | sudo sh" to
                        "curl -fsSL \"$relay/install.sh?$q\" | sudo sh"
            }
            out += CmdBlock("网关机安装命令", "在可访问外网的网关主机上执行", gwCmd)
            out += CmdBlock("内网机安装命令", "内网主机通过网关中继安装", inCmd)
        }
        else -> {
            val sj = if (mode == InstallMode.MULTI) parseMultiServers(multiText) else null
            val q = buildQuery(token, category, logPaths, sj)
            val (cmd, title, hint) = when (os) {
                InstallOs.WINDOWS -> Triple(
                    "${PS_TLS12}irm \"$server/install.ps1?$q\" | iex",
                    "PowerShell 安装命令",
                    "普通 PowerShell 即可（已内置 TLS 1.2，兼容 Windows Server 2012 R2）"
                )
                InstallOs.MACOS -> Triple(
                    "curl -fsSL \"$server/install.sh?$q\" | sh",
                    "终端一键安装",
                    "在 macOS 终端执行"
                )
                InstallOs.LINUX -> Triple(
                    "curl -fsSL \"$server/install.sh?$q\" | sudo sh",
                    "Linux 一键安装",
                    "需要 sudo 权限"
                )
            }
            out += CmdBlock(
                if (mode == InstallMode.MULTI) "多服务端安装命令" else title,
                if (mode == InstallMode.MULTI) "安装后向多个服务端同时上报" else hint,
                cmd
            )
        }
    }
    val uninstall = when (os) {
        InstallOs.WINDOWS -> "${PS_TLS12}irm \"$server/uninstall.ps1\" | iex"
        InstallOs.MACOS -> "curl -fsSL \"$server/uninstall.sh\" | sh"
        InstallOs.LINUX -> "curl -fsSL \"$server/uninstall.sh\" | sudo sh"
    }
    out += CmdBlock("卸载命令", "从目标主机移除 Agent", uninstall)
    return out
}

private fun copyText(context: Context, text: String) {
    val cm = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
    cm.setPrimaryClip(ClipData.newPlainText("install", text))
}
