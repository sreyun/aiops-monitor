package com.aiops.monitor.ui.screens

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import android.net.Uri
import android.util.Base64
import android.widget.Toast
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material.icons.filled.AttachFile
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.DeleteSweep
import androidx.compose.material.icons.filled.Mic
import androidx.compose.material.icons.filled.Stop
import androidx.compose.material.icons.filled.VolumeUp
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.content.ContextCompat
import androidx.lifecycle.viewmodel.compose.viewModel
import com.aiops.monitor.data.ApiClient
import com.aiops.monitor.data.models.AiFilePart
import com.aiops.monitor.data.models.AiImagePart
import com.aiops.monitor.data.models.AiMessage
import com.aiops.monitor.ui.AiVoiceHelper
import com.aiops.monitor.ui.components.MarkdownText
import com.aiops.monitor.ui.viewmodel.AiCopilotViewModel
import com.aiops.monitor.ui.viewmodel.aiCopilotViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

private data class PendingAiAttachment(
    val name: String,
    val mime: String,
    val kind: String, // image | file
    val data: String = "",
    val text: String = ""
)

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AiCopilotScreen(modifier: Modifier = Modifier, initialHostId: String? = null) {
    // Activity 级作用域：让 AlertsScreen 的“AI 诊断”按钮能把告警上下文放进**同一个**
    // VM 实例，本页 processPendingAlert() 才消费得到（VM 若各自 route 独立就接不上）。
    val vm: AiCopilotViewModel = aiCopilotViewModel()
    val messages by vm.messages.collectAsState()
    val loading by vm.loading.collectAsState()
    val activity by vm.activity.collectAsState()
    val suggestions by vm.suggestions.collectAsState()
    val dataSources by vm.dataSources.collectAsState()
    val context = LocalContext.current
    val scope = rememberCoroutineScope()

    var inputText by remember { mutableStateOf("") }
    var pending by remember { mutableStateOf<List<PendingAiAttachment>>(emptyList()) }
    var attaching by remember { mutableStateOf(false) }
    var listening by remember { mutableStateOf(false) }
    var speakingMsgId by remember { mutableStateOf<String?>(null) }
    val listState = rememberLazyListState()
    val voice = remember { AiVoiceHelper(context) }

    DisposableEffect(Unit) {
        voice.onListeningChanged = { listening = it }
        voice.onFinalResult = { text ->
            inputText = if (inputText.isBlank()) text else "$inputText $text"
        }
        voice.onSpeakingChanged = { on ->
            if (!on) speakingMsgId = null
        }
        voice.onError = { msg ->
            Toast.makeText(context, msg, Toast.LENGTH_SHORT).show()
        }
        onDispose { voice.release() }
    }

    val micPermission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
        if (granted) voice.startListening()
        else Toast.makeText(context, "需要麦克风权限才能语音输入", Toast.LENGTH_SHORT).show()
    }

    fun toggleMic() {
        if (listening) {
            voice.stopListening()
            return
        }
        val ok = ContextCompat.checkSelfPermission(context, Manifest.permission.RECORD_AUDIO) == PackageManager.PERMISSION_GRANTED
        if (ok) voice.startListening() else micPermission.launch(Manifest.permission.RECORD_AUDIO)
    }

    val pickFiles = rememberLauncherForActivityResult(ActivityResultContracts.OpenMultipleDocuments()) { uris ->
        if (uris.isEmpty()) return@rememberLauncherForActivityResult
        scope.launch {
            attaching = true
            try {
                val added = withContext(Dispatchers.IO) { prepareAiAttachments(context, uris) }
                pending = pending + added
            } finally {
                attaching = false
            }
        }
    }

    LaunchedEffect(Unit) {
        // 每次进入页面都与当前账号/服务器重新同步推荐问题和数据源能力。
        vm.refreshAiContext()
        // 优先消费挂起的告警诊断上下文（AlertsScreen 导航过来时设置）
        if (!vm.processPendingAlert() && !initialHostId.isNullOrBlank()) {
            vm.diagnose(initialHostId)
        }
    }

    // 不在每个流式字符到达时启动滚动动画，避免长回答造成持续重组和掉帧。
    LaunchedEffect(messages.size, loading) {
        if (messages.isNotEmpty()) {
            listState.scrollToItem(messages.size - 1)
        }
    }

    // 关键修复：imePadding() 必须在 fillMaxSize() 之前，
    // 否则键盘弹起时内容区不会正确收缩，导致输入框被遮挡
    Column(
        modifier = modifier
            .imePadding()
            .fillMaxSize()
            .background(MaterialTheme.colorScheme.background)
    ) {
        // ── 顶部栏 ──
        TopAppBar(
            title = {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(Icons.Default.AutoAwesome, contentDescription = null, tint = Color(0xFF4F7FFF), modifier = Modifier.size(20.dp))
                    Spacer(Modifier.width(8.dp))
                    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        Text("AI 运维助手", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface)
                        Text("知识检索 · 指标 · 告警 · 日志 · 外部数据源", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                }
            },
            actions = {
                IconButton(onClick = { vm.clearHistory() }, enabled = !loading) {
                    Icon(Icons.Default.DeleteSweep, contentDescription = "清空", tint = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            },
            colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background, titleContentColor = MaterialTheme.colorScheme.onSurface)
        )

        // ── 消息列表 ──
        LazyColumn(
            state = listState,
            modifier = Modifier
                .weight(1f)
                .fillMaxWidth()
                .padding(horizontal = 14.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
            contentPadding = PaddingValues(vertical = 12.dp)
        ) {
            if (messages.size <= 1) {
                item {
                    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        val lokiCount = dataSources.count { it.type.equals("loki", ignoreCase = true) }
                        val prometheusCount = dataSources.count { it.type.equals("prometheus", ignoreCase = true) }
                        Row(
                            modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                            horizontalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            SuggestionCapability("知识库检索")
                            SuggestionCapability("MCP 资料")
                            SuggestionCapability("Loki $lokiCount")
                            SuggestionCapability("Prometheus $prometheusCount")
                        }
                        Text("你可以直接这样问", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 11.sp)
                        suggestions.take(6).forEach { prompt ->
                            AssistChip(
                                onClick = { vm.sendMessage(prompt) },
                                enabled = !loading,
                                label = { Text(prompt, fontSize = 12.sp) }
                            )
                        }
                    }
                }
            }
            items(messages) { msg ->
                if (msg.type != "streaming" || msg.content.isNotBlank()) {
                    val key = "${msg.timestamp}|${msg.role}|${msg.content.hashCode()}"
                    ChatBubble(
                        msg = msg,
                        speaking = speakingMsgId == key,
                        onSpeak = {
                            if (speakingMsgId == key) {
                                voice.stopSpeaking()
                                speakingMsgId = null
                            } else {
                                speakingMsgId = key
                                voice.speak(msg.content)
                            }
                        }
                    )
                }
            }
            if (loading) {
                item {
                    LoadingBubble(activity)
                }
            }
        }

        // ── 输入区域 ── 高可见度设计，确保用户一眼可见
        Surface(
            modifier = Modifier.fillMaxWidth(),
            color = MaterialTheme.colorScheme.surfaceVariant,
            shadowElevation = 12.dp,
            tonalElevation = 4.dp
        ) {
            Column {
                // 顶部分隔线 — 明确区分聊天区和输入区
                HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant, thickness = 1.dp)

                Row(
                    modifier = Modifier
                        .padding(horizontal = 10.dp, vertical = 8.dp),
                    verticalAlignment = Alignment.Bottom,
                    horizontalArrangement = Arrangement.spacedBy(4.dp)
                ) {
                    IconButton(
                        onClick = {
                            pickFiles.launch(arrayOf(
                                "image/*",
                                "text/*",
                                "application/pdf",
                                "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
                                "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
                                "application/json",
                                "*/*"
                            ))
                        },
                        enabled = !loading && !attaching,
                        modifier = Modifier.size(36.dp)
                    ) {
                        Icon(Icons.Default.AttachFile, contentDescription = "添加图片或文件", tint = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.size(20.dp))
                    }
                    IconButton(
                        onClick = { toggleMic() },
                        enabled = !loading,
                        modifier = Modifier.size(36.dp)
                    ) {
                        Icon(
                            Icons.Default.Mic,
                            contentDescription = if (listening) "停止语音输入" else "语音输入",
                            tint = if (listening) Color(0xFFE84C4C) else MaterialTheme.colorScheme.onSurfaceVariant,
                            modifier = Modifier.size(20.dp)
                        )
                    }
                    OutlinedTextField(
                        value = inputText,
                        onValueChange = { inputText = it },
                        modifier = Modifier.weight(1f),
                        placeholder = {
                            Text(
                                if (listening) "正在聆听…" else "输入消息或添加附件…",
                                fontSize = 14.sp,
                                color = MaterialTheme.colorScheme.onSurfaceVariant
                            )
                        },
                        maxLines = 4,
                        minLines = 1,
                        shape = RoundedCornerShape(12.dp),
                        colors = OutlinedTextFieldDefaults.colors(
                            focusedBorderColor = Color(0xFF4F7FFF),
                            unfocusedBorderColor = MaterialTheme.colorScheme.outlineVariant,
                            focusedTextColor = MaterialTheme.colorScheme.onSurface,
                            unfocusedTextColor = MaterialTheme.colorScheme.onSurface,
                            focusedContainerColor = MaterialTheme.colorScheme.surfaceVariant,
                            unfocusedContainerColor = MaterialTheme.colorScheme.surfaceVariant,
                            cursorColor = Color(0xFF4F7FFF),
                            focusedPlaceholderColor = MaterialTheme.colorScheme.onSurfaceVariant,
                            unfocusedPlaceholderColor = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    )
                    // 小一号圆形发送，避免喧宾夺主
                    FilledIconButton(
                        onClick = {
                            if (loading) {
                                vm.stopGeneration()
                            } else if (inputText.isNotBlank() || pending.isNotEmpty()) {
                                val images = pending.filter { it.kind == "image" }.map { AiImagePart(it.mime, it.data) }
                                val files = pending.filter { it.kind == "file" }.map { AiFilePart(it.name, it.text) }
                                vm.sendMessage(inputText, images = images, files = files)
                                inputText = ""
                                pending = emptyList()
                            }
                        },
                        enabled = loading || inputText.isNotBlank() || pending.isNotEmpty(),
                        modifier = Modifier.size(36.dp),
                        shape = CircleShape,
                        colors = IconButtonDefaults.filledIconButtonColors(
                            containerColor = if (loading) Color(0xFFE84C4C) else Color(0xFF4F7FFF),
                            contentColor = Color.White,
                            disabledContainerColor = MaterialTheme.colorScheme.outlineVariant,
                            disabledContentColor = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    ) {
                        Icon(
                            if (loading) Icons.Default.Stop else Icons.AutoMirrored.Filled.Send,
                            contentDescription = if (loading) "停止生成" else "发送",
                            modifier = Modifier.size(16.dp)
                        )
                    }
                }
                if (pending.isNotEmpty() || attaching) {
                    Row(
                        Modifier
                            .fillMaxWidth()
                            .horizontalScroll(rememberScrollState())
                            .padding(start = 12.dp, end = 12.dp, bottom = 10.dp),
                        horizontalArrangement = Arrangement.spacedBy(6.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        if (attaching) {
                            CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp)
                            Text("解析附件…", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                        pending.forEachIndexed { idx, att ->
                            AssistChip(
                                onClick = { pending = pending.filterIndexed { i, _ -> i != idx } },
                                label = { Text("${if (att.kind == "image") "🖼" else "📄"} ${att.name}", fontSize = 11.sp, maxLines = 1) },
                                trailingIcon = { Icon(Icons.Default.Close, contentDescription = "移除", modifier = Modifier.size(14.dp)) }
                            )
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun SuggestionCapability(label: String) {
    Surface(
        color = MaterialTheme.colorScheme.surfaceVariant,
        shape = RoundedCornerShape(10.dp),
        border = androidx.compose.foundation.BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant)
    ) {
        Text(
            label,
            color = MaterialTheme.colorScheme.primary,
            fontSize = 10.sp,
            fontWeight = FontWeight.Medium,
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 5.dp)
        )
    }
}

@Composable
fun ChatBubble(
    msg: AiMessage,
    speaking: Boolean = false,
    onSpeak: (() -> Unit)? = null
) {
    val isAssistant = msg.role == "assistant"
    val isError = msg.type == "error"
    val alignment = if (isAssistant) Alignment.CenterStart else Alignment.CenterEnd

    val bgColor = when {
        isError -> MaterialTheme.colorScheme.errorContainer
        isAssistant -> MaterialTheme.colorScheme.surface
        else -> Color(0xFF4F7FFF)
    }
    val borderColor = if (isError) Color(0xFFEF4444).copy(alpha = 0.5f) else Color.Transparent

    // 时间格式化
    val timeStr = remember(msg.timestamp) {
        try {
            SimpleDateFormat("HH:mm", Locale.getDefault()).format(Date(msg.timestamp * 1000))
        } catch (_: Exception) { "" }
    }

    Column(horizontalAlignment = if (isAssistant) Alignment.Start else Alignment.End) {
        Box(Modifier.fillMaxWidth(), contentAlignment = alignment) {
            Column(
                modifier = Modifier
                    .fillMaxWidth(0.85f)
                    .background(bgColor, RoundedCornerShape(
                        topStart = 14.dp, topEnd = 14.dp,
                        bottomStart = if (isAssistant) 0.dp else 14.dp,
                        bottomEnd = if (isAssistant) 14.dp else 0.dp
                    ))
                    .then(
                        if (isError) Modifier.border(1.dp, borderColor, RoundedCornerShape(
                            topStart = 14.dp, topEnd = 14.dp,
                            bottomStart = 0.dp, bottomEnd = 14.dp
                        )) else Modifier
                    )
                    .padding(12.dp)
            ) {
                // 类型标签
                when (msg.type) {
                    "error" -> Text(
                        text = "⚠️ 错误",
                        style = MaterialTheme.typography.labelSmall,
                        color = Color(0xFFEF4444),
                        fontWeight = FontWeight.Bold,
                        modifier = Modifier.padding(bottom = 4.dp)
                    )
                    "diagnosis" -> Text(
                        text = "🔍 诊断报告",
                        style = MaterialTheme.typography.labelSmall,
                        color = Color(0xFF00A86B),
                        fontWeight = FontWeight.Bold,
                        modifier = Modifier.padding(bottom = 4.dp)
                    )
                    "orchestration" -> Text(
                        text = "⚙️ 编排结果",
                        style = MaterialTheme.typography.labelSmall,
                        color = Color(0xFF00A86B),
                        fontWeight = FontWeight.Bold,
                        modifier = Modifier.padding(bottom = 4.dp)
                    )
                }
                MarkdownText(
                    text = msg.content,
                    color = when {
                        isError -> MaterialTheme.colorScheme.onErrorContainer
                        isAssistant -> MaterialTheme.colorScheme.onSurface
                        else -> MaterialTheme.colorScheme.onPrimary
                    },
                    fontSize = MaterialTheme.typography.bodyMedium.fontSize,
                    lineHeight = 20.sp
                )
                // 朗读按钮：紧贴本条助手回复下方
                if (isAssistant && !isError && onSpeak != null && msg.content.isNotBlank() && msg.type != "streaming") {
                    TextButton(
                        onClick = onSpeak,
                        contentPadding = PaddingValues(horizontal = 8.dp, vertical = 0.dp),
                        modifier = Modifier.padding(top = 6.dp)
                    ) {
                        Icon(
                            Icons.Default.VolumeUp,
                            contentDescription = null,
                            modifier = Modifier.size(14.dp),
                            tint = if (speaking) Color(0xFF4F7FFF) else MaterialTheme.colorScheme.onSurfaceVariant
                        )
                        Spacer(Modifier.width(4.dp))
                        Text(
                            if (speaking) "朗读中…" else "朗读",
                            fontSize = 11.sp,
                            color = if (speaking) Color(0xFF4F7FFF) else MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    }
                }
            }
        }
        // 时间戳
        if (timeStr.isNotBlank()) {
            Text(
                text = timeStr,
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                fontSize = 10.sp,
                modifier = Modifier.padding(
                    start = if (isAssistant) 4.dp else 0.dp,
                    end = if (isAssistant) 0.dp else 4.dp,
                    top = 2.dp
                )
            )
        }
    }
}

@Composable
fun LoadingBubble(label: String = "AI 思考中") {
    // 动态跳动的小圆点：0→1→2→3 循环
    var dotCount by remember { mutableIntStateOf(0) }
    LaunchedEffect(Unit) {
        while (true) {
            delay(400L)
            dotCount = (dotCount + 1) % 4
        }
    }
    val dots = ".".repeat(dotCount)

    Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.CenterStart) {
        Row(
            modifier = Modifier
                .background(MaterialTheme.colorScheme.surface, RoundedCornerShape(topStart = 12.dp, topEnd = 12.dp, bottomEnd = 12.dp))
                .padding(horizontal = 16.dp, vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp, color = Color(0xFF4F7FFF))
            Spacer(Modifier.width(8.dp))
            Text(
                text = "$label$dots",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                // 固定三个点的宽度，避免文本跳动
                modifier = Modifier.widthIn(min = 0.dp)
            )
        }
    }
}

private suspend fun prepareAiAttachments(context: Context, uris: List<Uri>): List<PendingAiAttachment> {
    val out = mutableListOf<PendingAiAttachment>()
    val cr = context.contentResolver
    val maxBytes = 8 * 1024 * 1024 // 8MB 上限，防止大文件 OOM
    for (uri in uris) {
        val name = uri.lastPathSegment?.substringAfterLast('/') ?: "file"
        val mime = cr.getType(uri) ?: "application/octet-stream"
        val bytes = cr.openInputStream(uri)?.use { stream ->
            val buf = ByteArray(maxBytes + 1)
            var total = 0
            while (total <= maxBytes) {
                val n = stream.read(buf, total, buf.size - total)
                if (n <= 0) break
                total += n
            }
            if (total > maxBytes) null else buf.copyOf(total)
        } ?: continue
        if (mime.startsWith("image/") || name.matches(Regex(".*\\.(png|jpe?g|gif|webp|bmp)$", RegexOption.IGNORE_CASE))) {
            if (out.count { it.kind == "image" } >= 4) continue
            out += PendingAiAttachment(
                name = name,
                mime = if (mime.startsWith("image/")) mime else "image/jpeg",
                kind = "image",
                data = Base64.encodeToString(bytes, Base64.NO_WRAP)
            )
            continue
        }
        val lower = name.lowercase()
        val textExt = lower.endsWith(".txt") || lower.endsWith(".log") || lower.endsWith(".md") ||
            lower.endsWith(".json") || lower.endsWith(".yaml") || lower.endsWith(".yml") ||
            lower.endsWith(".csv") || lower.endsWith(".xml") || lower.endsWith(".sh") ||
            lower.endsWith(".py") || lower.endsWith(".go") || lower.endsWith(".js") ||
            lower.endsWith(".ts") || lower.endsWith(".conf") || lower.endsWith(".cfg") ||
            lower.endsWith(".ini") || mime.startsWith("text/")
        if (textExt) {
            out += PendingAiAttachment(name = name, mime = mime, kind = "file", text = bytes.toString(Charsets.UTF_8))
            continue
        }
        if (lower.endsWith(".pdf") || lower.endsWith(".docx") || lower.endsWith(".xlsx")) {
            try {
                val b64 = Base64.encodeToString(bytes, Base64.NO_WRAP)
                val resp = ApiClient.api.hermesParse(mapOf("name" to name, "mime" to mime, "data" to b64))
                val text = (resp["text"] ?: resp["content"])?.toString().orEmpty()
                if (text.isNotBlank()) {
                    out += PendingAiAttachment(name = name, mime = mime, kind = "file", text = text)
                }
            } catch (_: Exception) {
                // skip unreadable binary
            }
        }
    }
    return out
}
