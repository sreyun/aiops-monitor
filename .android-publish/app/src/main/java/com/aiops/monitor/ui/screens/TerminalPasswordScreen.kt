package com.aiops.monitor.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Visibility
import androidx.compose.material.icons.filled.VisibilityOff
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavHostController
import com.aiops.monitor.data.ApiClient
import com.aiops.monitor.ui.components.PrimaryButton
import com.aiops.monitor.ui.viewmodel.TerminalPasswordViewModel
import kotlinx.coroutines.delay
import kotlinx.coroutines.CancellationException

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TerminalPasswordScreen(navController: NavHostController, modifier: Modifier = Modifier) {
    val vm: TerminalPasswordViewModel = viewModel()
    val state by vm.state.collectAsState()
    val scheme = MaterialTheme.colorScheme

    var newPw by remember { mutableStateOf("") }
    var showNew by remember { mutableStateOf(false) }
    var confirm by remember { mutableStateOf("") }
    var showConfirm by remember { mutableStateOf(false) }
    var code by remember { mutableStateOf("") }
    var hasPassword by remember { mutableStateOf(false) }
    var statusLoaded by remember { mutableStateOf(false) }
    var statusError by remember { mutableStateOf<String?>(null) }
    var statusReload by remember { mutableIntStateOf(0) }

    val codeFocus = remember { FocusRequester() }

    LaunchedEffect(statusReload) {
        statusLoaded = false
        statusError = null
        try {
            check(ApiClient.isInitialized()) { "请先配置服务器并登录" }
            val s = ApiClient.api.terminalPasswordStatus()
            hasPassword = s["has_password"] == true
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            statusError = error.message ?: "无法读取终端密码状态"
        } finally {
            statusLoaded = true
        }
    }

    LaunchedEffect(state) {
        when (val s = state) {
            is TerminalPasswordViewModel.SetState.Success -> {
                delay(900)
                navController.popBackStack()
            }
            is TerminalPasswordViewModel.SetState.MfaRequired -> {
                // Drop leftover login-password from the shared field before asking for TOTP
                if (s.clearCode) code = ""
                codeFocus.requestFocus()
            }
            is TerminalPasswordViewModel.SetState.Error -> {
                if (s.clearCode) code = ""
            }
            else -> {}
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(if (hasPassword) "修改终端密码" else "设置终端密码") },
                navigationIcon = {
                    IconButton(onClick = { navController.popBackStack() }) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "返回")
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = scheme.background)
            )
        },
        containerColor = scheme.background,
        contentColor = scheme.onBackground
    ) { padding ->
        Column(
            modifier = modifier.fillMaxSize().padding(padding).verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(14.dp)
        ) {
            if (!statusLoaded) {
                Text("正在读取终端密码状态…", color = scheme.onSurfaceVariant)
            } else if (statusError != null) {
                Card(
                    modifier = Modifier.fillMaxWidth(),
                    colors = CardDefaults.cardColors(containerColor = scheme.surface)
                ) {
                    Column(Modifier.padding(20.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                        Text(statusError ?: "读取失败", color = scheme.error)
                        PrimaryButton(text = "重试", onClick = { statusReload++ })
                    }
                }
            } else {
            Card(
                modifier = Modifier.fillMaxWidth(),
                shape = RoundedCornerShape(20.dp),
                colors = CardDefaults.cardColors(containerColor = scheme.surface),
                elevation = CardDefaults.cardElevation(defaultElevation = 2.dp)
            ) {
                Column(Modifier.padding(horizontal = 20.dp, vertical = 22.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
                    Text(
                        "终端二次密码与登录密码相互独立，用于打开远程终端（最高权限）。建议设置强密码。",
                        style = MaterialTheme.typography.bodyMedium,
                        color = scheme.onSurfaceVariant
                    )

                    PasswordField(
                        value = newPw,
                        onValueChange = { newPw = it; vm.reset() },
                        label = "新终端密码",
                        show = showNew,
                        onToggleShow = { showNew = !showNew },
                        imeAction = ImeAction.Next
                    )

                    PasswordField(
                        value = confirm,
                        onValueChange = { confirm = it; vm.reset() },
                        label = "确认新密码",
                        show = showConfirm,
                        onToggleShow = { showConfirm = !showConfirm },
                        imeAction = ImeAction.Next
                    )

                    // 强度提示（与服务端 validateTerminalPassword 对齐：≥8 位 + 大写 + 小写 + 数字 + 特殊符）
                    val unmet = passwordUnmet(newPw)
                    if (newPw.isNotBlank()) {
                        Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                            unmet.forEach { req ->
                                Text("· $req", style = MaterialTheme.typography.bodySmall, color = scheme.error)
                            }
                            if (unmet.isEmpty() && confirm != newPw) {
                                Text("· 两次输入的密码不一致", style = MaterialTheme.typography.bodySmall, color = scheme.error)
                            }
                        }
                    }

                    // 修改已存在的密码时，需要验证凭据（MFA 动态码 或 登录密码，由服务端决定）
                    if (hasPassword) {
                        val mfaMode = state is TerminalPasswordViewModel.SetState.MfaRequired
                        if (mfaMode) {
                            OutlinedTextField(
                                value = code,
                                onValueChange = { raw ->
                                    code = raw.filter { it.isDigit() }.take(6)
                                },
                                modifier = Modifier.fillMaxWidth().focusRequester(codeFocus),
                                label = { Text("动态验证码（MFA）") },
                                singleLine = true,
                                keyboardOptions = KeyboardOptions(
                                    capitalization = KeyboardCapitalization.None,
                                    autoCorrectEnabled = false,
                                    keyboardType = KeyboardType.NumberPassword,
                                    imeAction = ImeAction.Done
                                ),
                                colors = OutlinedTextFieldDefaults.colors()
                            )
                        } else {
                            PasswordField(
                                value = code,
                                onValueChange = { code = it; vm.reset() },
                                label = "验证凭据（登录密码）",
                                show = false,
                                onToggleShow = { },
                                modifier = Modifier.focusRequester(codeFocus),
                                imeAction = ImeAction.Done,
                                keyboardType = KeyboardType.Password
                            )
                        }
                    }

                    if (state is TerminalPasswordViewModel.SetState.MfaRequired) {
                        Text(
                            (state as TerminalPasswordViewModel.SetState.MfaRequired).message
                                ?: "账户已启用 MFA，请输入 6 位动态口令后再次点击保存",
                            style = MaterialTheme.typography.bodySmall,
                            color = scheme.primary
                        )
                    }

                    if (state is TerminalPasswordViewModel.SetState.Error) {
                        Text(
                            (state as TerminalPasswordViewModel.SetState.Error).message,
                            style = MaterialTheme.typography.bodySmall,
                            color = scheme.error
                        )
                    }

                    if (state is TerminalPasswordViewModel.SetState.Success) {
                        Text("设置成功，正在返回…", style = MaterialTheme.typography.bodySmall, color = scheme.secondary)
                    }

                    val unmetNow = passwordUnmet(newPw)
                    val mfaMode = state is TerminalPasswordViewModel.SetState.MfaRequired
                    val verifyOk = !hasPassword || if (mfaMode) {
                        code.filter { it.isDigit() }.length == 6
                    } else {
                        code.isNotBlank()
                    }
                    val canSave = newPw.isNotBlank() && confirm == newPw && unmetNow.isEmpty()
                            && verifyOk
                            && state !is TerminalPasswordViewModel.SetState.Loading

                    PrimaryButton(
                        text = if (state is TerminalPasswordViewModel.SetState.Loading) "保存中…" else "保存",
                        onClick = {
                            val verify = if (mfaMode) code.filter { it.isDigit() }.take(6) else code
                            vm.set(newPw, verify)
                        },
                        enabled = canSave
                    )
                }
            }
            }
            Spacer(Modifier.height(4.dp))
            Text(
                "提示：在 APP 内设置后，本机会以同一（无自动纠错的）输入通道保存并校验，确保「设置」与「终端验证」使用完全一致的密码。",
                style = MaterialTheme.typography.bodySmall,
                color = scheme.onSurfaceVariant
            )
        }
    }
}

@Composable
private fun PasswordField(
    value: String,
    onValueChange: (String) -> Unit,
    label: String,
    show: Boolean,
    onToggleShow: () -> Unit,
    modifier: Modifier = Modifier,
    imeAction: ImeAction = ImeAction.Done,
    keyboardType: KeyboardType = KeyboardType.Password
) {
    val scheme = MaterialTheme.colorScheme
    OutlinedTextField(
        value = value,
        onValueChange = onValueChange,
        label = { Text(label) },
        singleLine = true,
        modifier = modifier.fillMaxWidth(),
        visualTransformation = if (show) VisualTransformation.None else PasswordVisualTransformation(),
        // 关键：关掉自动纠错与首字母大写，避免软键盘把密码悄悄改写（与登录/终端验证框一致）
        keyboardOptions = KeyboardOptions(
            keyboardType = keyboardType,
            capitalization = KeyboardCapitalization.None,
            autoCorrectEnabled = false,
            imeAction = imeAction
        ),
        trailingIcon = if (keyboardType == KeyboardType.Password) {
            {
                IconButton(onClick = onToggleShow) {
                    Icon(
                        if (show) Icons.Default.VisibilityOff else Icons.Default.Visibility,
                        contentDescription = if (show) "隐藏" else "显示"
                    )
                }
            }
        } else null,
        shape = RoundedCornerShape(12.dp),
        colors = OutlinedTextFieldDefaults.colors(
            focusedBorderColor = scheme.primary,
            cursorColor = scheme.primary
        )
    )
}

/** 返回尚未满足的密码强度要求（与服务端 validateTerminalPassword 对齐）。 */
private fun passwordUnmet(p: String): List<String> {
    val reqs = mutableListOf<String>()
    if (p.length < 8) reqs.add("至少 8 位")
    if (!p.any { it.isUpperCase() }) reqs.add("需包含大写字母")
    if (!p.any { it.isLowerCase() }) reqs.add("需包含小写字母")
    if (!p.any { it.isDigit() }) reqs.add("需包含数字")
    if (!p.any { !it.isLetterOrDigit() }) reqs.add("需包含特殊符号")
    return reqs
}
