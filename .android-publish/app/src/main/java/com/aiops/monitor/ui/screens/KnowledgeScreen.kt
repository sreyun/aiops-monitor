package com.aiops.monitor.ui.screens

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.navigation.NavHostController
import com.aiops.monitor.data.ApiClient
import com.aiops.monitor.data.models.HermesRule
import com.aiops.monitor.data.models.HermesTemplate
import com.aiops.monitor.ui.components.StateBox
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun KnowledgeScreen(navController: NavHostController, modifier: Modifier = Modifier) {
    var seg by remember { mutableIntStateOf(0) }
    var rules by remember { mutableStateOf<List<HermesRule>>(emptyList()) }
    var templates by remember { mutableStateOf<List<HermesTemplate>>(emptyList()) }
    var loading by remember { mutableStateOf(true) }
    var error by remember { mutableStateOf<String?>(null) }
    val snack = remember { SnackbarHostState() }
    val scope = rememberCoroutineScope()

    fun reload() {
        scope.launch {
            loading = true
            error = null
            try {
                rules = ApiClient.api.hermesRules()
                templates = runCatching { ApiClient.api.hermesTemplates() }.getOrDefault(emptyList())
            } catch (e: Exception) {
                error = e.message ?: "加载失败（需 PostgreSQL / Hermes）"
            } finally {
                loading = false
            }
        }
    }

    LaunchedEffect(Unit) { reload() }

    Scaffold(
        modifier = modifier,
        topBar = {
            Column {
                TopAppBar(
                    title = { Text("知识库 / Hermes", fontWeight = FontWeight.Bold) },
                    navigationIcon = {
                        IconButton(onClick = { navController.popBackStack() }) {
                            Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回")
                        }
                    },
                    actions = {
                        IconButton(onClick = { reload() }) { Icon(Icons.Default.Refresh, "刷新") }
                    },
                    colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background)
                )
                Row(Modifier.padding(horizontal = 16.dp, vertical = 6.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    FilterChip(selected = seg == 0, onClick = { seg = 0 }, label = { Text("规则") })
                    FilterChip(selected = seg == 1, onClick = { seg = 1 }, label = { Text("模板") })
                }
            }
        },
        snackbarHost = { SnackbarHost(snack) },
        containerColor = MaterialTheme.colorScheme.background
    ) { padding ->
        when {
            loading && rules.isEmpty() && templates.isEmpty() -> Box(Modifier.fillMaxSize().padding(padding), contentAlignment = Alignment.Center) {
                CircularProgressIndicator()
            }
            error != null && rules.isEmpty() && templates.isEmpty() -> StateBox(error ?: "加载失败", Modifier.fillMaxSize().padding(padding), ::reload)
            seg == 0 && rules.isEmpty() -> StateBox("暂无 Hermes 规则\n可在 Web 端创建，或在 AI 对话中上传文档解析", Modifier.fillMaxSize().padding(padding))
            seg == 1 && templates.isEmpty() -> StateBox("暂无模板", Modifier.fillMaxSize().padding(padding))
            seg == 0 -> LazyColumn(
                Modifier.fillMaxSize().padding(padding),
                contentPadding = PaddingValues(16.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                items(rules, key = { it.id }) { r ->
                    Card(
                        Modifier.fillMaxWidth(),
                        shape = RoundedCornerShape(12.dp),
                        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface)
                    ) {
                        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Column(Modifier.weight(1f)) {
                                    Text(r.name.ifBlank { "规则 #${r.id}" }, fontWeight = FontWeight.Bold)
                                    if (r.description.isNotBlank()) Text(r.description, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                    Text("优先级 ${r.priority} · ${if (r.enabled) "启用" else "停用"}", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                }
                                Switch(
                                    checked = r.enabled,
                                    onCheckedChange = { en ->
                                        scope.launch {
                                            try {
                                                ApiClient.api.upsertHermesRule(r.copy(enabled = en))
                                                reload()
                                            } catch (e: Exception) {
                                                snack.showSnackbar(e.message ?: "更新失败")
                                            }
                                        }
                                    }
                                )
                                IconButton(onClick = {
                                    scope.launch {
                                        try {
                                            ApiClient.api.deleteHermesRule(r.id)
                                            reload()
                                            snack.showSnackbar("已删除")
                                        } catch (e: Exception) {
                                            snack.showSnackbar(e.message ?: "删除失败")
                                        }
                                    }
                                }) {
                                    Icon(Icons.Default.Delete, "删除", tint = MaterialTheme.colorScheme.error)
                                }
                            }
                        }
                    }
                }
            }
            else -> LazyColumn(
                Modifier.fillMaxSize().padding(padding),
                contentPadding = PaddingValues(16.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                items(templates, key = { it.id }) { t ->
                    Card(
                        Modifier.fillMaxWidth(),
                        shape = RoundedCornerShape(12.dp),
                        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface)
                    ) {
                        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                            Text(t.name.ifBlank { "模板 #${t.id}" }, fontWeight = FontWeight.Bold)
                            if (t.description.isNotBlank()) Text(t.description, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            if (t.content.isNotBlank()) Text(t.content.take(200), fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    }
                }
            }
        }
    }
}
