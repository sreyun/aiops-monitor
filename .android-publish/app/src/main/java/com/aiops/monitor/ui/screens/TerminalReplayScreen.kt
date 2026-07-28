package com.aiops.monitor.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavHostController
import com.aiops.monitor.data.models.TermFrame
import com.aiops.monitor.data.models.TerminalSessionInfo
import com.aiops.monitor.ui.Routes
import com.aiops.monitor.ui.components.*
import com.aiops.monitor.ui.viewmodel.TerminalReplayViewModel

/* ───────── 会话列表 ───────── */

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TerminalSessionsScreen(navController: NavHostController, modifier: Modifier = Modifier) {
    val vm: TerminalReplayViewModel = viewModel()
    val sessions by vm.sessions.collectAsState()
    val loading by vm.loading.collectAsState()
    val error by vm.error.collectAsState()

    LaunchedEffect(Unit) { vm.loadSessions() }

    Scaffold(
        modifier = modifier,
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            TopAppBar(
                title = {
                    Column {
                        Text("终端会话回放", fontWeight = FontWeight.Bold)
                        Text("查看已录制的终端会话（需终端二次密码）", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                },
                navigationIcon = {
                    IconButton(onClick = { navController.popBackStack() }) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回", tint = MaterialTheme.colorScheme.onSurface)
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background)
            )
        }
    ) { padding ->
        Box(Modifier.fillMaxSize().padding(padding)) {
            when {
                loading && sessions.isEmpty() -> LoadingBox(Modifier.fillMaxSize())
                error != null && sessions.isEmpty() -> StateBox("加载失败：$error", Modifier.fillMaxSize(), onRetry = vm::loadSessions)
                sessions.isEmpty() -> StateBox("暂无终端会话记录", Modifier.fillMaxSize())
                else -> LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    items(sessions, key = { it.id }) { s -> SessionCard(s) { navController.navigate(Routes.terminalReplay(s.id)) } }
                }
            }
        }
    }
}

@Composable
private fun SessionCard(s: TerminalSessionInfo, onClick: () -> Unit) {
    Card(
        Modifier.fillMaxWidth().clickable(onClick = onClick),
        shape = RoundedCornerShape(12.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 1.dp)
    ) {
        Row(Modifier.fillMaxWidth().padding(12.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(3.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(s.hostname.ifBlank { s.host_id }, fontSize = 14.sp, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    if (s.active) StatusPill("进行中", Color(0xFF00A86B))
                }
                Text("操作员 ${s.operator.ifBlank { "-" }} · ${s.ip.ifBlank { "-" }}", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1)
                Text("${fmtSessTime(s.created_at)} · ${s.frames} 帧" + if (s.observers > 0) " · ${s.observers} 观察者" else "", fontSize = 10.sp, color = Color(0xFF8A93A3))
            }
            Icon(Icons.Default.PlayArrow, "回放", tint = Color(0xFF4F7FFF))
        }
    }
}

/* ───────── 回放（录制帧转录） ───────── */

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TerminalReplayScreen(sessionId: String, navController: NavHostController, modifier: Modifier = Modifier) {
    val vm: TerminalReplayViewModel = viewModel()
    val frames by vm.frames.collectAsState()
    val loading by vm.loading.collectAsState()
    val error by vm.error.collectAsState()
    val needVerify by vm.needTerminalVerify.collectAsState()

    LaunchedEffect(sessionId) { vm.loadReplay(sessionId) }

    val transcript = remember(frames) { decodeTranscript(frames) }

    Scaffold(
        modifier = modifier,
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            TopAppBar(
                title = { Text("会话回放", fontWeight = FontWeight.Bold) },
                navigationIcon = {
                    IconButton(onClick = { navController.popBackStack() }) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回", tint = MaterialTheme.colorScheme.onSurface)
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background)
            )
        }
    ) { padding ->
        Box(Modifier.fillMaxSize().padding(padding)) {
            when {
                needVerify -> Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.Center, horizontalAlignment = Alignment.CenterHorizontally) {
                    Text("需要终端二次验证", fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface)
                    Spacer(Modifier.height(8.dp))
                    Text("终端录制含完整 Shell I/O（可能有他人输入的密钥），需先设置/验证终端二次密码。", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(bottom = 16.dp))
                    Button(onClick = { navController.navigate(Routes.TERMINAL_PASSWORD) }) { Text("去设置终端密码") }
                }
                loading && frames.isEmpty() -> LoadingBox(Modifier.fillMaxSize())
                error != null -> StateBox("加载失败：$error", Modifier.fillMaxSize(), onRetry = { vm.loadReplay(sessionId) })
                frames.isEmpty() -> StateBox("该会话无录制内容", Modifier.fillMaxSize())
                else -> SelectionContainer {
                    Text(
                        transcript.ifBlank { "(无输出)" },
                        modifier = Modifier.fillMaxSize().background(Color(0xFF0B1020)).verticalScroll(rememberScrollState()).padding(12.dp),
                        color = Color(0xFFCDD6E4),
                        fontFamily = FontFamily.Monospace,
                        fontSize = 12.sp
                    )
                }
            }
        }
    }
}

// 拼接 output 帧并逐字符去除 ANSI CSI/OSC 转义（ESC=27），避免 regex 字面量转义的坑。
private fun decodeTranscript(frames: List<TermFrame>): String {
    val raw = StringBuilder()
    for (f in frames) {
        if (f.type != "output") continue
        val bytes = try { android.util.Base64.decode(f.data, android.util.Base64.DEFAULT) } catch (_: Exception) { continue }
        raw.append(String(bytes, Charsets.UTF_8))
    }
    val esc = 27.toChar()
    val out = StringBuilder(raw.length)
    var i = 0
    while (i < raw.length) {
        val c = raw[i]
        if (c == esc && i + 1 < raw.length) {
            val n = raw[i + 1]
            when (n) {
                '[' -> { i += 2; while (i < raw.length && raw[i] !in '@'..'~') i++; i++ }   // CSI
                ']' -> { i += 2; while (i < raw.length && raw[i].code != 7) i++; i++ }        // OSC → BEL
                else -> i += 2                                                                 // 其它双字符转义
            }
            continue
        }
        if (c.code == 13) {                       // CR / CRLF → LF
            out.append('\n'); i++
            if (i < raw.length && raw[i] == '\n') i++
            continue
        }
        if (c.code == 8 || c.code == 7) { i++; continue }  // 退格/响铃丢弃
        out.append(c); i++
    }
    return out.toString()
}

private fun fmtSessTime(sec: Long): String {
    if (sec <= 0) return "-"
    val millis = if (sec > 1_000_000_000_000L) sec else sec * 1000
    return java.text.SimpleDateFormat("MM-dd HH:mm", java.util.Locale.getDefault()).format(java.util.Date(millis))
}
