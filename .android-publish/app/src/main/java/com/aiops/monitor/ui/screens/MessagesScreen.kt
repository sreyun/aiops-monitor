package com.aiops.monitor.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.DoneAll
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
import androidx.navigation.NavHostController
import com.aiops.monitor.data.models.Message
import com.aiops.monitor.ui.components.*
import com.aiops.monitor.ui.viewmodel.MessagesViewModel

private fun msgColor(level: String): Color = when (level.lowercase()) {
    "critical" -> Color(0xFFEF4444); "warning" -> Color(0xFFF59E0B)
    "success" -> Color(0xFF00A86B); else -> Color(0xFF4F7FFF)
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MessagesScreen(navController: NavHostController, modifier: Modifier = Modifier) {
    val vm: MessagesViewModel = viewModel()
    val messages by vm.messages.collectAsState()
    val unread by vm.unread.collectAsState()
    val loading by vm.loading.collectAsState()
    val error by vm.error.collectAsState()

    LaunchedEffect(Unit) { vm.load() }

    Scaffold(
        modifier = modifier,
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            TopAppBar(
                title = {
                    Column {
                        Text("消息中心", fontWeight = FontWeight.Bold)
                        Text(if (unread > 0) "$unread 条未读" else "全部已读", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                },
                navigationIcon = {
                    IconButton(onClick = { navController.popBackStack() }) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回", tint = MaterialTheme.colorScheme.onSurface)
                    }
                },
                actions = {
                    if (unread > 0) {
                        TextButton(onClick = { vm.markAllRead() }) {
                            Icon(Icons.Default.DoneAll, null, modifier = Modifier.size(16.dp))
                            Spacer(Modifier.width(4.dp))
                            Text("全部已读")
                        }
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background)
            )
        }
    ) { padding ->
        Box(Modifier.fillMaxSize().padding(padding)) {
            when {
                loading && messages.isEmpty() -> LoadingBox(Modifier.fillMaxSize())
                error != null && messages.isEmpty() -> StateBox("加载失败：$error", Modifier.fillMaxSize(), onRetry = vm::load)
                messages.isEmpty() -> StateBox("暂无消息", Modifier.fillMaxSize())
                else -> LazyColumn(
                    Modifier.fillMaxSize(),
                    contentPadding = PaddingValues(16.dp),
                    verticalArrangement = Arrangement.spacedBy(10.dp)
                ) {
                    items(messages, key = { it.id }) { m -> MessageCard(m) { if (!m.read) vm.markRead(listOf(m.id)) } }
                }
            }
        }
    }
}

@Composable
private fun MessageCard(m: Message, onClick: () -> Unit) {
    val color = msgColor(m.level)
    Card(
        Modifier.fillMaxWidth().clickable(onClick = onClick),
        shape = RoundedCornerShape(12.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = if (m.read) 0.dp else 1.dp)
    ) {
        Row(Modifier.fillMaxWidth().padding(12.dp), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            Box(Modifier.padding(top = 4.dp).size(8.dp).clip(CircleShape).background(if (m.read) Color.Transparent else color))
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(3.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    StatusPill(msgTypeLabel(m.type), color)
                    Text(m.title, fontSize = 14.sp, fontWeight = if (m.read) FontWeight.Normal else FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
                }
                if (m.body.isNotBlank()) Text(m.body, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 3, overflow = TextOverflow.Ellipsis)
                Text(relTimeMs(m.ts), fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
    }
}

private fun msgTypeLabel(t: String): String = when (t) {
    "incident" -> "事件"; "alert" -> "告警"; "slo" -> "SLO"; "remediation" -> "修复"
    "ai" -> "AI"; "ticket" -> "工单"; "system" -> "系统"; else -> t.ifBlank { "消息" }
}

private fun relTimeMs(ts: Long): String {
    if (ts <= 0) return ""
    val millis = if (ts > 1_000_000_000_000L) ts else ts * 1000
    val diff = (System.currentTimeMillis() - millis) / 1000
    return when {
        diff < 60 -> "刚刚"; diff < 3600 -> "${diff / 60}分钟前"
        diff < 86400 -> "${diff / 3600}小时前"; else -> "${diff / 86400}天前"
    }
}
