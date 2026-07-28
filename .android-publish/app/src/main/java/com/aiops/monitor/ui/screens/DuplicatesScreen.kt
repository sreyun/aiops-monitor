package com.aiops.monitor.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
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
import com.aiops.monitor.data.models.DupGroup
import com.aiops.monitor.ui.components.*
import kotlinx.coroutines.launch
import com.aiops.monitor.ui.viewmodel.DuplicatesViewModel

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DuplicatesScreen(navController: NavHostController, modifier: Modifier = Modifier) {
    val vm: DuplicatesViewModel = viewModel()
    val groups by vm.groups.collectAsState()
    val loading by vm.loading.collectAsState()
    val busy by vm.busy.collectAsState()
    val notice by vm.notice.collectAsState()
    val error by vm.error.collectAsState()
    val snackbar = remember { SnackbarHostState() }
    val scope = rememberCoroutineScope()
    var confirm by remember { mutableStateOf(false) }

    LaunchedEffect(Unit) { vm.load() }
    LaunchedEffect(notice) { notice?.let { scope.launch { snackbar.showSnackbar(it) }; vm.clearNotice() } }

    val staleTotal = groups.sumOf { it.stale }

    Scaffold(
        modifier = modifier,
        containerColor = MaterialTheme.colorScheme.background,
        snackbarHost = { SnackbarHost(snackbar) },
        topBar = {
            TopAppBar(
                title = {
                    Column {
                        Text("重复主机", fontWeight = FontWeight.Bold)
                        Text("按机器指纹识别 · 重装 Agent 产生的同机旧身份", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                },
                navigationIcon = {
                    IconButton(onClick = { navController.popBackStack() }) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回", tint = MaterialTheme.colorScheme.onSurface)
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background)
            )
        },
        floatingActionButton = {
            if (staleTotal > 0) {
                ExtendedFloatingActionButton(
                    onClick = { confirm = true },
                    containerColor = Color(0xFFEF4444),
                    contentColor = Color.White
                ) {
                    if (busy) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp, color = Color.White)
                    else Text("清理 $staleTotal 个可清理项")
                }
            }
        }
    ) { padding ->
        Box(Modifier.fillMaxSize().padding(padding)) {
            when {
                loading && groups.isEmpty() -> LoadingBox(Modifier.fillMaxSize())
                error != null && groups.isEmpty() -> StateBox("加载失败：$error", Modifier.fillMaxSize(), onRetry = vm::load)
                groups.isEmpty() -> StateBox("无重复主机\n所有主机身份唯一，无需清理", Modifier.fillMaxSize())
                else -> LazyColumn(
                    Modifier.fillMaxSize(),
                    contentPadding = PaddingValues(16.dp),
                    verticalArrangement = Arrangement.spacedBy(12.dp)
                ) {
                    items(groups, key = { it.group }) { g -> DupGroupCard(g) }
                }
            }
        }
    }

    if (confirm) {
        AlertDialog(
            onDismissRequest = { confirm = false },
            title = { Text("清理重复主机？") },
            text = { Text("将删除 $staleTotal 个「非当前身份且已离线」的重复主机记录。当前在用身份与在线主机不受影响，此操作不可撤销。") },
            confirmButton = {
                Button(onClick = { confirm = false; vm.cleanup() }, colors = ButtonDefaults.buttonColors(containerColor = Color(0xFFEF4444))) { Text("清理") }
            },
            dismissButton = { TextButton(onClick = { confirm = false }) { Text("取消") } }
        )
    }
}

@Composable
private fun DupGroupCard(g: DupGroup) {
    Card(
        Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(14.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 1.dp)
    ) {
        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(g.hostname.ifBlank { "未知主机" }, fontWeight = FontWeight.Bold, fontSize = 15.sp, color = MaterialTheme.colorScheme.onSurface, modifier = Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis)
                val hosts = g.hosts.orEmpty()
                Text("${hosts.size} 条记录", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant, thickness = 0.5.dp)
            g.hosts.orEmpty().forEach { h ->
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    StatusDot(if (h.online) Color(0xFF00A86B) else Color(0xFF8A93A3), size = 8.dp)
                    Column(Modifier.weight(1f)) {
                        Text(h.id, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurface, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Text("${h.ip.ifBlank { "无 IP" }} · ${if (h.online) "在线" else "离线"}", fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    when {
                        h.current -> StatusPill("当前", Color(0xFF00A86B))
                        h.stale -> StatusPill("可清理", Color(0xFFEF4444))
                        else -> StatusPill("保留", Color(0xFF8A93A3))
                    }
                }
            }
        }
    }
}
