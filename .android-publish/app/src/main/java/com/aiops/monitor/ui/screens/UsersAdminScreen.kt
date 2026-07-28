package com.aiops.monitor.ui.screens

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.navigation.NavHostController
import com.aiops.monitor.data.ApiClient
import com.aiops.monitor.data.models.AccountUser
import com.aiops.monitor.data.models.CreateUserRequest
import com.aiops.monitor.data.models.UpdateUserRequest
import com.aiops.monitor.ui.components.StateBox
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun UsersAdminScreen(navController: NavHostController, modifier: Modifier = Modifier) {
    var users by remember { mutableStateOf<List<AccountUser>>(emptyList()) }
    var loading by remember { mutableStateOf(true) }
    var error by remember { mutableStateOf<String?>(null) }
    var showCreate by remember { mutableStateOf(false) }
    var editing by remember { mutableStateOf<AccountUser?>(null) }
    var deleting by remember { mutableStateOf<AccountUser?>(null) }
    val snack = remember { SnackbarHostState() }
    val scope = rememberCoroutineScope()

    fun reload() {
        scope.launch {
            loading = true
            error = null
            try {
                users = ApiClient.api.users()
            } catch (e: Exception) {
                error = e.message ?: "加载失败（需管理员权限）"
            } finally {
                loading = false
            }
        }
    }

    LaunchedEffect(Unit) { reload() }

    Scaffold(
        modifier = modifier,
        topBar = {
            TopAppBar(
                title = { Text("用户管理", fontWeight = FontWeight.Bold) },
                navigationIcon = {
                    IconButton(onClick = { navController.popBackStack() }) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回")
                    }
                },
                actions = {
                    IconButton(onClick = { showCreate = true }) {
                        Icon(Icons.Default.Add, "新建用户")
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background)
            )
        },
        snackbarHost = { SnackbarHost(snack) },
        containerColor = MaterialTheme.colorScheme.background
    ) { padding ->
        when {
            loading && users.isEmpty() -> Box(Modifier.fillMaxSize().padding(padding), contentAlignment = Alignment.Center) {
                CircularProgressIndicator()
            }
            error != null && users.isEmpty() -> StateBox(error ?: "加载失败", Modifier.fillMaxSize().padding(padding), ::reload)
            else -> LazyColumn(
                Modifier.fillMaxSize().padding(padding),
                contentPadding = PaddingValues(16.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                item {
                    Text("管理员可创建/改角色/删除用户。删除后对方 App 会话会立即。", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                items(users, key = { it.username }) { u ->
                    Card(
                        Modifier.fillMaxWidth(),
                        shape = RoundedCornerShape(12.dp),
                        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface)
                    ) {
                        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Column(Modifier.weight(1f)) {
                                    Text(u.display_name.ifBlank { u.username }, fontWeight = FontWeight.Bold)
                                    Text("${u.username} · ${u.role}" + if (u.mfa_enabled) " · MFA" else "", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                    if (u.email.isNotBlank()) Text(u.email, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                }
                                TextButton(onClick = { editing = u }) { Text("编辑") }
                                IconButton(onClick = { deleting = u }) {
                                    Icon(Icons.Default.Delete, "删除", tint = MaterialTheme.colorScheme.error)
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    if (showCreate) {
        UserEditorDialog(
            title = "新建用户",
            initial = null,
            onDismiss = { showCreate = false },
            onSave = { req, _ ->
                scope.launch {
                    try {
                        ApiClient.api.createUser(req)
                        showCreate = false
                        reload()
                        snack.showSnackbar("已创建 ${req.username}")
                    } catch (e: Exception) {
                        snack.showSnackbar(e.message ?: "创建失败")
                    }
                }
            }
        )
    }
    editing?.let { u ->
        UserEditorDialog(
            title = "编辑 ${u.username}",
            initial = u,
            onDismiss = { editing = null },
            onSave = { _, update ->
                scope.launch {
                    try {
                        ApiClient.api.updateUser(u.username, update!!)
                        editing = null
                        reload()
                        snack.showSnackbar("已保存")
                    } catch (e: Exception) {
                        snack.showSnackbar(e.message ?: "保存失败")
                    }
                }
            }
        )
    }
    deleting?.let { u ->
        AlertDialog(
            onDismissRequest = { deleting = null },
            title = { Text("删除用户") },
            text = { Text("确认删除 ${u.username}？其会话将被踢出。") },
            confirmButton = {
                TextButton(onClick = {
                    scope.launch {
                        try {
                            ApiClient.api.deleteUser(u.username)
                            deleting = null
                            reload()
                            snack.showSnackbar("已删除")
                        } catch (e: Exception) {
                            snack.showSnackbar(e.message ?: "删除失败")
                        }
                    }
                }) { Text("删除", color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = { TextButton(onClick = { deleting = null }) { Text("取消") } }
        )
    }
}

@Composable
private fun UserEditorDialog(
    title: String,
    initial: AccountUser?,
    onDismiss: () -> Unit,
    onSave: (CreateUserRequest, UpdateUserRequest?) -> Unit
) {
    var username by remember { mutableStateOf(initial?.username.orEmpty()) }
    var password by remember { mutableStateOf("") }
    var display by remember { mutableStateOf(initial?.display_name.orEmpty()) }
    var email by remember { mutableStateOf(initial?.email.orEmpty()) }
    var role by remember { mutableStateOf(initial?.role?.ifBlank { "viewer" } ?: "viewer") }
    val isEdit = initial != null

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                if (!isEdit) {
                    OutlinedTextField(username, { username = it }, label = { Text("用户名") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                    OutlinedTextField(password, { password = it }, label = { Text("密码") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                }
                OutlinedTextField(display, { display = it }, label = { Text("显示名") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                OutlinedTextField(email, { email = it }, label = { Text("邮箱") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    listOf("viewer", "operator", "admin").forEach { r ->
                        FilterChip(selected = role == r, onClick = { role = r }, label = { Text(r) })
                    }
                }
            }
        },
        confirmButton = {
            TextButton(onClick = {
                if (isEdit) {
                    onSave(
                        CreateUserRequest("", ""),
                        UpdateUserRequest(display_name = display, email = email, role = role)
                    )
                } else {
                    onSave(
                        CreateUserRequest(username = username.trim(), password = password, display_name = display, email = email, role = role),
                        null
                    )
                }
            }) { Text("保存") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("取消") } }
    )
}
