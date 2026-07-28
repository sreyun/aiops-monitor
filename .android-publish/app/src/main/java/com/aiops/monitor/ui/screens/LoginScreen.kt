package com.aiops.monitor.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.Visibility
import androidx.compose.material.icons.filled.VisibilityOff
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusDirection
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavHostController
import kotlinx.coroutines.launch
import com.aiops.monitor.data.ApiClient
import com.aiops.monitor.data.store.SettingsStore
import com.aiops.monitor.ui.Routes
import com.aiops.monitor.ui.components.PrimaryButton
import com.aiops.monitor.ui.viewmodel.AuthViewModel

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun LoginScreen(navController: NavHostController, settingsStore: SettingsStore) {
    val vm: AuthViewModel = viewModel()
    val state by vm.state.collectAsState()
    val focusManager = LocalFocusManager.current

    var username by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var showPwd by remember { mutableStateOf(false) }
    var mfaCode by remember { mutableStateOf("") }
    var showMfaDialog by remember { mutableStateOf(false) }
    var mfaUser by remember { mutableStateOf("") }
    var mfaPass by remember { mutableStateOf("") }

    // 隐藏的环境切换入口：右下角连点 3 次唤出。地址管理以此为准（更名/编辑/删除/切换）。
    val scope = rememberCoroutineScope()
    var showEnvSwitcher by remember { mutableStateOf(false) }
    var tapCount by remember { mutableStateOf(0) }
    var lastTapAt by remember { mutableStateOf(0L) }
    var envError by remember { mutableStateOf<String?>(null) }
    val servers by settingsStore.serverList.collectAsState(initial = emptyList())
    val currentUrl by settingsStore.baseUrl.collectAsState(initial = null)

    LaunchedEffect(state) {
        when (val s = state) {
            is AuthViewModel.LoginState.MfaRequired -> {
                mfaUser = s.username
                mfaPass = s.password
                showMfaDialog = true
                // Critical: wipe OTP after failure/replay so a consumed code cannot be resubmitted
                // (that was burning the same TOTP for Web login as well).
                if (s.clearCode) {
                    mfaCode = ""
                }
            }
            is AuthViewModel.LoginState.Success -> {
                // 必须先复位「未授权单飞」锁，否则 injectCookieHeader 会拒绝写入旧会话清理后的新 cookie
                ApiClient.markAuthorized()
                val cookie = ApiClient.cookieHeader()
                if (cookie.isNotBlank()) {
                    settingsStore.saveSessionCookie(cookie)
                }
                navController.navigate(Routes.DASHBOARD) {
                    popUpTo(Routes.LOGIN) { inclusive = true }
                }
            }
            else -> {}
        }
    }

    val mfaVisible = showMfaDialog && (state is AuthViewModel.LoginState.MfaRequired || state is AuthViewModel.LoginState.Loading)
    val mfaState = state as? AuthViewModel.LoginState.MfaRequired

    fun applyServerRoot(raw: String, label: String = "", editingUrl: String? = null) {
        scope.launch {
            envError = null
            try {
                val normalizedBase = ApiClient.normalizeBaseUrl(raw)
                val serverRoot = normalizedBase.removeSuffix("api/v1/").trimEnd('/')
                if (editingUrl != null) {
                    settingsStore.updateServer(editingUrl, serverRoot, label)
                } else {
                    settingsStore.saveServer(serverRoot, label)
                }
                ApiClient.clearSession()
                ApiClient.init(serverRoot)
                settingsStore.clearSessionCookie()
                vm.reset()
            } catch (e: Exception) {
                envError = e.message ?: "服务器地址无效"
            }
        }
    }

    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(MaterialTheme.colorScheme.background)
            .safeDrawingPadding()
            .imePadding(),
        contentAlignment = Alignment.Center
    ) {
        // 登录页不显示右上角设置入口；服务器地址统一由右下角隐藏热区管理。

        Column(
            modifier = Modifier
                .widthIn(max = 480.dp)
                .fillMaxWidth()
                .padding(horizontal = 24.dp, vertical = 12.dp)
                .verticalScroll(rememberScrollState()),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(14.dp)
        ) {
            Box(
                modifier = Modifier
                    .size(56.dp)
                    .clip(RoundedCornerShape(16.dp))
                    .background(Color(0xFF4F7FFF).copy(alpha = 0.12f)),
                contentAlignment = Alignment.Center
            ) {
                Icon(Icons.Default.Lock, contentDescription = null, tint = Color(0xFF4F7FFF), modifier = Modifier.size(28.dp))
            }

            Column(horizontalAlignment = Alignment.CenterHorizontally) {
                Text("智能运维", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Black, color = MaterialTheme.colorScheme.onSurface)
                Text("AIOps 智能运维监控平台", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }

            Card(
                modifier = Modifier.fillMaxWidth(),
                shape = RoundedCornerShape(18.dp),
                colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
                elevation = CardDefaults.cardElevation(defaultElevation = 0.dp)
            ) {
                Column(
                    Modifier.fillMaxWidth().padding(20.dp),
                    verticalArrangement = Arrangement.spacedBy(14.dp)
                ) {
                    Text("账号登录", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface)

                    OutlinedTextField(
                        value = username,
                        onValueChange = { username = it },
                        modifier = Modifier.fillMaxWidth(),
                        label = { Text("用户名") },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.None, autoCorrectEnabled = false, imeAction = ImeAction.Next),
                        keyboardActions = KeyboardActions(onNext = { focusManager.moveFocus(FocusDirection.Down) }),
                        shape = RoundedCornerShape(12.dp),
                        colors = loginFieldColors()
                    )

                    OutlinedTextField(
                        value = password,
                        onValueChange = { password = it },
                        modifier = Modifier.fillMaxWidth(),
                        label = { Text("密码") },
                        singleLine = true,
                        visualTransformation = if (showPwd) VisualTransformation.None else PasswordVisualTransformation(),
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, capitalization = KeyboardCapitalization.None, autoCorrectEnabled = false, imeAction = ImeAction.Done),
                        keyboardActions = KeyboardActions(onDone = {
                            focusManager.clearFocus()
                            vm.login(username, password)
                        }),
                        trailingIcon = {
                            IconButton({ showPwd = !showPwd }) {
                                Icon(if (showPwd) Icons.Default.VisibilityOff else Icons.Default.Visibility, contentDescription = null, tint = MaterialTheme.colorScheme.onSurfaceVariant)
                            }
                        },
                        shape = RoundedCornerShape(12.dp),
                        colors = loginFieldColors()
                    )

                    if (state is AuthViewModel.LoginState.Error) {
                        Text((state as AuthViewModel.LoginState.Error).message, color = Color(0xFFEF4444), style = MaterialTheme.typography.bodySmall)
                    }

                    PrimaryButton(
                        text = if (state is AuthViewModel.LoginState.Loading) "正在验证..." else "登 录",
                        onClick = { focusManager.clearFocus(); vm.login(username, password) },
                        loading = state is AuthViewModel.LoginState.Loading,
                        enabled = username.isNotBlank() && password.isNotBlank() && !currentUrl.isNullOrBlank()
                    )
                }
            }

            Text(
                when {
                    currentUrl.isNullOrBlank() -> "尚未配置服务器，请连点右下角设置访问地址"
                    ApiClient.serverHost.startsWith("https://") -> "HTTPS 安全连接 · ${ApiClient.serverHost}"
                    else -> "HTTP 明文连接 · ${ApiClient.serverHost}"
                },
                style = MaterialTheme.typography.labelSmall,
                color = when {
                    currentUrl.isNullOrBlank() -> Color(0xFFF59E0B)
                    ApiClient.serverHost.startsWith("https://") -> MaterialTheme.colorScheme.onSurfaceVariant
                    else -> Color(0xFFF59E0B)
                }
            )
        }

        // 右下角隐藏热区：1.5s 内连点 3 次唤出环境切换。透明、无视觉反馈。
        Box(
            modifier = Modifier
                .align(Alignment.BottomEnd)
                .size(64.dp)
                .clickable(
                    indication = null,
                    interactionSource = remember { MutableInteractionSource() }
                ) {
                    val now = System.currentTimeMillis()
                    tapCount = if (now - lastTapAt < 1500L) tapCount + 1 else 1
                    lastTapAt = now
                    if (tapCount >= 3) {
                        tapCount = 0
                        envError = null
                        showEnvSwitcher = true
                    }
                }
        )
    }

    if (showEnvSwitcher) {
        EnvSwitcherDialog(
            servers = servers,
            currentUrl = currentUrl,
            error = envError,
            onSwitch = { entry ->
                scope.launch {
                    envError = null
                    try {
                        ApiClient.init(entry.url)
                        ApiClient.clearSession()
                        settingsStore.switchServer(entry.url)
                        settingsStore.clearSessionCookie()
                        vm.reset()
                        showEnvSwitcher = false
                    } catch (e: Exception) {
                        envError = e.message ?: "切换失败"
                    }
                }
            },
            onSave = { url, label, editingUrl ->
                applyServerRoot(url, label, editingUrl)
            },
            onDelete = { entry ->
                scope.launch {
                    envError = null
                    val wasActive = entry.url.trimEnd('/') == currentUrl?.trimEnd('/')
                    val next = settingsStore.removeServer(entry.url)
                    if (wasActive) {
                        ApiClient.clearSession()
                        settingsStore.clearSessionCookie()
                        if (next != null) {
                            try {
                                ApiClient.init(next)
                            } catch (e: Exception) {
                                envError = e.message ?: "备用地址无效"
                            }
                        }
                        vm.reset()
                    }
                }
            },
            onDismiss = { showEnvSwitcher = false },
        )
    }

    if (mfaVisible) {
        AlertDialog(
            onDismissRequest = { },
            containerColor = MaterialTheme.colorScheme.surface,
            shape = RoundedCornerShape(16.dp),
            confirmButton = {
                val digits = mfaCode.filter { it.isDigit() }
                Button(
                    onClick = { vm.login(mfaUser, mfaPass, digits) },
                    enabled = digits.length == 6 && state !is AuthViewModel.LoginState.Loading,
                    shape = RoundedCornerShape(10.dp)
                ) {
                    if (state is AuthViewModel.LoginState.Loading) {
                        CircularProgressIndicator(modifier = Modifier.size(18.dp), strokeWidth = 2.dp, color = MaterialTheme.colorScheme.onPrimary)
                    } else {
                        Text("验证", fontSize = 13.sp)
                    }
                }
            },
            dismissButton = {
                TextButton(onClick = {
                    showMfaDialog = false
                    mfaCode = ""
                    vm.reset()
                }) { Text("取消", fontSize = 13.sp) }
            },
            title = { Text("双因素认证", fontSize = 16.sp, fontWeight = FontWeight.Bold) },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    Text(
                        if (mfaState?.errorCode == "totp_replay")
                            "该口令已使用过。请等待 Authenticator 刷新为新的 6 位数字后再验证。"
                        else
                            "请输入 Authenticator 当前显示的 6 位动态口令（每 30 秒刷新一次，请勿重复提交同一口令）",
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                    OutlinedTextField(
                        value = mfaCode,
                        onValueChange = { raw ->
                            // Allow brief paste with spaces; keep digits only, max 6
                            mfaCode = raw.filter { c -> c.isDigit() }.take(6)
                        },
                        modifier = Modifier.fillMaxWidth(),
                        label = { Text("动态口令") },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(
                            keyboardType = KeyboardType.NumberPassword,
                            imeAction = ImeAction.Done
                        ),
                        keyboardActions = KeyboardActions(onDone = {
                            val digits = mfaCode.filter { it.isDigit() }
                            if (digits.length == 6 && state !is AuthViewModel.LoginState.Loading) {
                                vm.login(mfaUser, mfaPass, digits)
                            }
                        }),
                        shape = RoundedCornerShape(12.dp),
                        colors = loginFieldColors()
                    )
                    if (mfaState?.error != null) {
                        Text(mfaState.error, color = Color(0xFFEF4444), style = MaterialTheme.typography.bodySmall)
                    }
                }
            }
        )
    }
}

@Composable
internal fun loginFieldColors() = OutlinedTextFieldDefaults.colors(
    focusedBorderColor = Color(0xFF4F7FFF),
    unfocusedBorderColor = MaterialTheme.colorScheme.outlineVariant,
    focusedContainerColor = MaterialTheme.colorScheme.background,
    unfocusedContainerColor = MaterialTheme.colorScheme.background,
    focusedLabelColor = Color(0xFF4F7FFF)
)
