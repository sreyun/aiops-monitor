package com.aiops.monitor.ui.screens

import android.content.pm.ActivityInfo
import androidx.activity.ComponentActivity
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.isAltPressed
import androidx.compose.ui.input.key.isCtrlPressed
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.navigation.NavHostController
import com.aiops.monitor.data.ApiClient
import com.aiops.monitor.data.TerminalClient
import com.aiops.monitor.data.models.TerminalPasswordVerify
import com.aiops.monitor.data.terminal.TerminalInputEncoder
import com.aiops.monitor.data.terminal.TerminalSnapshot
import com.aiops.monitor.ui.components.StatusDot
import com.aiops.monitor.ui.components.StatusPill
import retrofit2.HttpException
import kotlinx.coroutines.delay
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TerminalScreen(hostId: String, navController: NavHostController, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val activity = context as? ComponentActivity

    // ── 状态管理 ──────────────────────────────
    var showPwDialog by remember { mutableStateOf(true) }
    var pw by remember { mutableStateOf("") }
    var showPw by remember { mutableStateOf(false) }
    var pwError by remember { mutableStateOf<String?>(null) }
    var pwVerifying by remember { mutableStateOf(false) }

    var verifyTrigger by remember { mutableIntStateOf(0) }
    var pendingPw by remember { mutableStateOf("") }

    var client by remember { mutableStateOf<TerminalClient?>(null) }
    val terminalSnapshot = client?.screen?.collectAsState(initial = TerminalSnapshot())?.value ?: TerminalSnapshot()
    val status = client?.status?.collectAsState(initial = "idle")?.value ?: "idle"
    val connected = status == "connected"
    val reconnecting = status.startsWith("reconnecting:")

    // ── 输入栏状态 ────────────────────────────
    val inputSentinel = "\u2060"
    var inputValue by remember {
        mutableStateOf(TextFieldValue(inputSentinel, selection = TextRange(inputSentinel.length)))
    }
    var ctrlArmed by remember { mutableStateOf(false) }
    var altArmed by remember { mutableStateOf(false) }
    val focusRequester = remember { FocusRequester() }
    val scrollState = rememberScrollState()
    val horizontalScrollState = rememberScrollState()
    val density = LocalDensity.current
    val focusManager = LocalFocusManager.current
    val keyboardController = LocalSoftwareKeyboardController.current
    val keyboardScope = rememberCoroutineScope()
    val imeBottom = WindowInsets.ime.getBottom(density)
    val imeVisible = imeBottom > 0
    var viewportSize by remember { mutableStateOf(IntSize.Zero) }
    val terminalColumns = remember(viewportSize.width, density) {
        (viewportSize.width / with(density) { 7.2.dp.toPx() }).toInt().coerceIn(40, 240)
    }
    val terminalRows = remember(viewportSize.height, density) {
        (viewportSize.height / with(density) { 16.dp.toPx() }).toInt().coerceIn(8, 120)
    }
    val cursorTransition = rememberInfiniteTransition(label = "terminal-cursor")
    val cursorAlpha by cursorTransition.animateFloat(
        initialValue = 1f,
        targetValue = 0.08f,
        animationSpec = infiniteRepeatable(animation = tween(520), repeatMode = RepeatMode.Reverse),
        label = "terminal-cursor-alpha"
    )

    fun resetInputBridge() {
        inputValue = TextFieldValue(inputSentinel, selection = TextRange(inputSentinel.length))
    }

    fun sendInput(raw: String, applyArmedModifiers: Boolean = true) {
        if (raw.isEmpty()) return
        val encoded = if (applyArmedModifiers) {
            TerminalInputEncoder.encode(raw, control = ctrlArmed, alt = altArmed)
        } else {
            TerminalInputEncoder.encode(raw)
        }
        if (client?.sendInput(encoded) == true && applyArmedModifiers) {
            ctrlArmed = false
            altArmed = false
        }
        resetInputBridge()
    }

    fun activateKeyboard() {
        if (!connected) return
        // IME 收起后隐藏输入桥通常仍处于 focused，单纯 requestFocus 不会再次触发键盘。
        // 先释放焦点，再在下一帧重新聚焦并显式 show，兼容手势返回键和工具栏收起两种路径。
        focusManager.clearFocus(force = true)
        keyboardScope.launch {
            delay(48L)
            focusRequester.requestFocus()
            delay(48L)
            keyboardController?.show()
        }
    }

    // 输出更新、键盘开合或横竖屏导致可视高度变化时，都将提示符保持在键盘上方。
    LaunchedEffect(terminalSnapshot.revision, imeBottom, viewportSize.height) {
        delay(24L) // 等待 Compose 完成本轮尺寸与 scroll range 计算
        scrollState.scrollTo(scrollState.maxValue)
    }

    // 软键盘会显著改变终端行数；同步更新远端 PTY，避免输出仍按旧高度绘制到键盘后方。
    LaunchedEffect(status, terminalColumns, terminalRows) {
        if (connected) {
            delay(80L)
            client?.sendResize(terminalColumns, terminalRows)
        }
    }

    // 连接建立后自动获取输入焦点
    LaunchedEffect(client, status) {
        if (client != null && connected) {
            delay(300L)
            focusRequester.requestFocus()
        }
    }

    LaunchedEffect(status) {
        if (status == "need_terminal_password") {
            client?.close()
            client = null
            pwError = "终端授权已失效，请重新验证"
            showPwDialog = true
        }
    }

    // ── 生命周期管理 ──────────────────────────
    DisposableEffect(activity) {
        val previousOrientation = activity?.requestedOrientation
        activity?.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED
        onDispose {
            if (previousOrientation != null) activity.requestedOrientation = previousOrientation
        }
    }

    val lifecycleOwner = LocalLifecycleOwner.current
    DisposableEffect(client, lifecycleOwner) {
        val activeClient = client
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) activeClient?.ensureConnected()
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose {
            lifecycleOwner.lifecycle.removeObserver(observer)
            activeClient?.close()
        }
    }

    // ── 密码验证 ──────────────────────────────
    LaunchedEffect(verifyTrigger) {
        if (verifyTrigger == 0) return@LaunchedEffect
        val password = pendingPw
        pwVerifying = true
        try {
            val resp = ApiClient.api.verifyTerminalPassword(TerminalPasswordVerify(password))
            if (resp["ok"] == true) {
                val tc = TerminalClient(ApiClient.baseUrl, hostId, ApiClient.cookieHeader())
                tc.connect(cols = 100, rows = 35)
                client = tc
                showPwDialog = false
                pwError = null
                pw = ""
                pendingPw = ""
            } else {
                pwError = "密码错误，请重试"
            }
        } catch (e: HttpException) {
            pwError = "验证失败: ${e.code()}"
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            pwError = "网络错误: ${e.message}"
        } finally {
            pwVerifying = false
        }
    }

    Scaffold(
        modifier = modifier,
        topBar = {
            TopAppBar(
                title = {
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        Text("远程终端", fontWeight = FontWeight.Bold, style = MaterialTheme.typography.titleMedium, color = MaterialTheme.colorScheme.onSurface)
                        StatusPill(
                            when {
                                connected -> "已连接"
                                reconnecting -> "自动重连"
                                status == "connecting" -> "连接中"
                                status == "closed:session_ended" -> "会话结束"
                                else -> "未连接"
                            },
                            when {
                                connected -> Color(0xFF00A86B)
                                reconnecting || status == "connecting" -> Color(0xFFF59E0B)
                                else -> Color.Gray
                            }
                        )
                    }
                },
                navigationIcon = {
                    IconButton(onClick = { navController.popBackStack() }) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "返回", tint = MaterialTheme.colorScheme.onSurface)
                    }
                },
                actions = {
                    IconButton(onClick = {
                        if (activity != null) {
                            activity.requestedOrientation = if (activity.requestedOrientation == ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE)
                                ActivityInfo.SCREEN_ORIENTATION_PORTRAIT else ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE
                        }
                    }) {
                        Icon(Icons.Default.ScreenRotation, contentDescription = "旋转", tint = MaterialTheme.colorScheme.onSurface)
                    }
                    TextButton(onClick = {
                        if (client != null) client?.reconnectNow() else showPwDialog = true
                    }) { Text("重连", color = Color(0xFF4F7FFF)) }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background, titleContentColor = MaterialTheme.colorScheme.onSurface)
            )
        },
        containerColor = Color.Black
    ) { padding ->
        Column(
            Modifier
                .fillMaxSize()
                .padding(padding)
                .consumeWindowInsets(padding)
                .imePadding()
                .background(Color.Black)
        ) {
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .weight(1f)
                    .onSizeChanged { viewportSize = it }
                    .clickable { activateKeyboard() }
            ) {
                // VT 屏幕输出：光标插入真实网格位置，全屏程序不会再被展平成乱码文本。
                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .verticalScroll(scrollState)
                        .padding(horizontal = 9.dp, vertical = 7.dp)
                ) {
                    val terminalText = buildAnnotatedString {
                        val visibleText = if (terminalSnapshot.text.isEmpty() && !showPwDialog) {
                            if (reconnecting) "网络波动，正在自动恢复终端会话…" else "连接成功，等待 Shell 初始化…"
                        } else terminalSnapshot.text
                        val cursorOffset = terminalSnapshot.cursorOffset?.coerceIn(0, visibleText.length)
                        if (connected && terminalSnapshot.cursorVisible && !showPwDialog && cursorOffset != null) {
                            append(visibleText.substring(0, cursorOffset))
                            withStyle(SpanStyle(color = Color(0xFF7CFF9B).copy(alpha = cursorAlpha))) {
                                append("▋")
                            }
                            append(visibleText.substring(cursorOffset))
                        } else {
                            append(visibleText)
                        }
                    }
                    Text(
                        text = terminalText,
                        color = Color(0xFF00A86B),
                        fontFamily = FontFamily.Monospace,
                        fontSize = 12.sp,
                        lineHeight = 16.sp,
                        softWrap = false,
                        modifier = Modifier.horizontalScroll(horizontalScrollState)
                    )
                    if (!connected && status !in setOf("idle", "connecting") && terminalSnapshot.text.isNotEmpty()) {
                        Text(
                            text = "\n${terminalStatusMessage(status)}",
                            color = Color(0xFFF59E0B),
                            fontFamily = FontFamily.Monospace,
                            fontSize = 11.sp
                        )
                    }
                }

                // 哨兵式输入桥：每次提交真实的 IME 编辑事务，支持删除、同长替换、中文组合和特殊字符。
                BasicTextField(
                    value = inputValue,
                    onValueChange = { newValue ->
                        if (newValue.composition != null) {
                            // 等待拼音/手写等输入法完成组词，避免把中间拼音逐字发送到 Shell。
                            inputValue = newValue
                        } else {
                            val committed = newValue.text.replace(inputSentinel, "")
                            when {
                                newValue.text.isEmpty() -> sendInput("\u007f", applyArmedModifiers = false)
                                committed.isNotEmpty() -> sendInput(committed)
                                else -> resetInputBridge()
                            }
                        }
                    },
                    modifier = Modifier
                        .size(2.dp)
                        .align(Alignment.BottomStart)
                        .focusRequester(focusRequester)
                        .onPreviewKeyEvent { event ->
                            if (event.type != KeyEventType.KeyDown) return@onPreviewKeyEvent false
                            val sequence = when (event.key) {
                                Key.Backspace -> "\u007f"
                                Key.Delete -> "\u001b[3~"
                                Key.Enter -> "\r"
                                Key.Tab -> "\t"
                                Key.Escape -> "\u001b"
                                Key.DirectionUp -> "\u001b[A"
                                Key.DirectionDown -> "\u001b[B"
                                Key.DirectionLeft -> "\u001b[D"
                                Key.DirectionRight -> "\u001b[C"
                                Key.MoveHome -> "\u001b[H"
                                Key.MoveEnd -> "\u001b[F"
                                Key.PageUp -> "\u001b[5~"
                                Key.PageDown -> "\u001b[6~"
                                else -> null
                            }
                            if (sequence != null) {
                                sendInput(sequence, applyArmedModifiers = false)
                                true
                            } else if (event.isCtrlPressed || event.isAltPressed) {
                                val unicode = event.nativeKeyEvent.unicodeChar
                                if (unicode > 0) {
                                    client?.sendInput(
                                        TerminalInputEncoder.encode(
                                            String(Character.toChars(unicode)),
                                            control = event.isCtrlPressed,
                                            alt = event.isAltPressed
                                        )
                                    )
                                    true
                                } else false
                            } else false
                        },
                    textStyle = TextStyle(fontFamily = FontFamily.Monospace, fontSize = 12.sp, color = Color.Transparent),
                    cursorBrush = SolidColor(Color.Transparent),
                    singleLine = false,
                    keyboardOptions = KeyboardOptions(
                        keyboardType = KeyboardType.Ascii,
                        capitalization = KeyboardCapitalization.None,
                        autoCorrectEnabled = false,
                        imeAction = ImeAction.Default
                    )
                )
            }

            if (connected) {
                Surface(color = MaterialTheme.colorScheme.background) {
                    Row(
                        Modifier.fillMaxWidth().height(38.dp).horizontalScroll(rememberScrollState()).padding(horizontal = 5.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(3.dp)
                    ) {
                        TerminalKey("ESC") { sendInput("\u001b", false) }
                        TerminalKey("TAB") { sendInput("\t", false) }
                        TerminalKey("CTRL", selected = ctrlArmed) { ctrlArmed = !ctrlArmed }
                        TerminalKey("ALT", selected = altArmed) { altArmed = !altArmed }
                        TerminalKey("CTRL+C", danger = true) { sendInput("\u0003", false) }
                        TerminalKey("CTRL+D", danger = true) { sendInput("\u0004", false) }
                        TerminalKey("↑") { sendInput("\u001b[A", false) }
                        TerminalKey("↓") { sendInput("\u001b[B", false) }
                        TerminalKey("←") { sendInput("\u001b[D", false) }
                        TerminalKey("→") { sendInput("\u001b[C", false) }
                        TerminalKey("HOME") { sendInput("\u001b[H", false) }
                        TerminalKey("END") { sendInput("\u001b[F", false) }
                        TerminalKey("/") { sendInput("/", false) }
                        TerminalKey("-") { sendInput("-", false) }
                        TerminalKey("_") { sendInput("_", false) }
                        TerminalKey("|") { sendInput("|", false) }
                        TerminalKey("~") { sendInput("~", false) }
                        TerminalKey(":") { sendInput(":", false) }
                    }
                }
            }

            // 始终位于 IME 上方的输入状态栏，可明确看到当前视口没有被 26 键键盘遮挡。
            Surface(color = MaterialTheme.colorScheme.background, tonalElevation = 2.dp) {
                Row(
                    Modifier.fillMaxWidth().height(38.dp).clickable {
                        activateKeyboard()
                    }.padding(start = 10.dp, end = 4.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    StatusDot(
                        when {
                            connected -> Color(0xFF00A86B)
                            reconnecting || status == "connecting" -> Color(0xFFF59E0B)
                            else -> Color.Gray
                        },
                        size = 7.dp
                    )
                    Spacer(Modifier.width(7.dp))
                    Text(
                        when {
                            reconnecting -> terminalStatusMessage(status)
                            status == "connecting" -> "正在建立加密终端通道…"
                            status == "closed:session_ended" -> "Shell 已退出 · 点击右上角重连"
                            status == "unauthorized" -> "登录会话已失效，请重新登录"
                            !connected -> "终端未连接 · 点击右上角重连"
                            imeVisible -> "键盘输入已激活 · UTF-8 · ${terminalColumns}×${terminalRows}"
                            else -> "点击终端输入 · VT100/xterm · ${terminalColumns}×${terminalRows}"
                        },
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        fontFamily = FontFamily.Monospace,
                        fontSize = 10.sp,
                        modifier = Modifier.weight(1f),
                        maxLines = 1
                    )
                    if (imeVisible) {
                        IconButton(onClick = {
                            keyboardController?.hide()
                            focusManager.clearFocus(force = true)
                        }, modifier = Modifier.size(34.dp)) {
                            Icon(Icons.Default.KeyboardHide, "收起键盘", tint = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.size(18.dp))
                        }
                    } else if (connected) {
                        IconButton(onClick = { activateKeyboard() }, modifier = Modifier.size(34.dp)) {
                            Icon(Icons.Default.Keyboard, "唤起26键键盘", tint = Color(0xFF75A7FF), modifier = Modifier.size(19.dp))
                        }
                    }
                }
            }
        }

        // ── 密码验证弹窗 ──────────────────────
        if (showPwDialog) {
            AlertDialog(
                onDismissRequest = {},
                containerColor = MaterialTheme.colorScheme.surface,
                shape = RoundedCornerShape(16.dp),
                title = { Text("终端验证", fontSize = 16.sp, fontWeight = FontWeight.Bold) },
                text = {
                    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                        Text("该操作具有最高权限，请输入终端二次密码：", fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        OutlinedTextField(
                            value = pw,
                            onValueChange = { pw = it; pwError = null },
                            label = { Text("终端密码") },
                            singleLine = true,
                            visualTransformation = if (showPw) VisualTransformation.None else PasswordVisualTransformation(),
                            keyboardOptions = KeyboardOptions(
                                keyboardType = KeyboardType.Password,
                                autoCorrectEnabled = false,
                                capitalization = KeyboardCapitalization.None,
                                imeAction = ImeAction.Done
                            ),
                            keyboardActions = KeyboardActions(onDone = {
                                if (pw.isNotBlank() && !pwVerifying) {
                                    pendingPw = pw
                                    verifyTrigger++
                                }
                            }),
                            trailingIcon = {
                                IconButton({ showPw = !showPw }) {
                                    Icon(if (showPw) Icons.Default.VisibilityOff else Icons.Default.Visibility, contentDescription = null)
                                }
                            },
                            isError = pwError != null,
                            shape = RoundedCornerShape(12.dp),
                            colors = OutlinedTextFieldDefaults.colors(focusedBorderColor = Color(0xFF4F7FFF))
                        )
                        pwError?.let { Text(it, color = Color(0xFFEF4444), style = MaterialTheme.typography.bodySmall) }
                    }
                },
                confirmButton = {
                    Button(
                        onClick = {
                            if (pw.isNotBlank() && !pwVerifying) {
                                pendingPw = pw
                                verifyTrigger++
                            }
                        },
                        enabled = !pwVerifying && pw.isNotBlank(),
                        shape = RoundedCornerShape(10.dp),
                        colors = ButtonDefaults.buttonColors(containerColor = Color(0xFF4F7FFF))
                    ) {
                        if (pwVerifying) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp, color = MaterialTheme.colorScheme.onSurface)
                        else Text("验证并连接", fontSize = 13.sp)
                    }
                },
                dismissButton = {
                    TextButton(onClick = { navController.popBackStack() }) { Text("取消", fontSize = 13.sp) }
                }
            )
        }
    }
}

@Composable
private fun TerminalKey(label: String, danger: Boolean = false, selected: Boolean = false, onClick: () -> Unit) {
    TextButton(
        onClick = onClick,
        modifier = Modifier.height(30.dp),
        contentPadding = PaddingValues(horizontal = 10.dp, vertical = 0.dp),
        colors = ButtonDefaults.textButtonColors(
            contentColor = when {
                danger -> Color(0xFFFF6B6B)
                selected -> Color(0xFF75A7FF)
                else -> MaterialTheme.colorScheme.onSurface
            },
            containerColor = if (selected) MaterialTheme.colorScheme.primaryContainer else Color.Transparent
        )
    ) {
        Text(label, fontFamily = FontFamily.Monospace, fontSize = 10.sp, fontWeight = FontWeight.Bold)
    }
}

private fun terminalStatusMessage(status: String): String = when {
    status.startsWith("reconnecting:") -> {
        val parts = status.split(':')
        val attempt = parts.getOrNull(1)?.toIntOrNull() ?: 1
        val seconds = parts.getOrNull(2)?.toLongOrNull() ?: 0
        if (seconds > 0) "网络中断 · 第 ${attempt} 次自动重连（${seconds}s）" else "正在重新建立终端会话…"
    }
    status == "closed:session_ended" -> "Shell 会话已正常结束"
    status == "unauthorized" -> "登录会话已失效"
    status.startsWith("error:") -> "连接失败：${status.substringAfter(':')}"
    status.startsWith("closed") -> "终端连接已关闭"
    else -> "等待终端连接"
}
