package com.aiops.monitor.ui.components

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.aiops.monitor.data.ApiClient
import com.aiops.monitor.data.models.AiAssistRequest
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import org.json.JSONObject
import java.io.IOException

/**
 * 薄客户端接入 POST /ai/assist：打开即分析给定 context，流式展示结论。
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AiAssistSheet(
    title: String,
    task: String,
    context: String,
    onDismiss: () -> Unit
) {
    val scope = rememberCoroutineScope()
    var answer by remember { mutableStateOf("") }
    var ragHint by remember { mutableStateOf("") }
    var error by remember { mutableStateOf<String?>(null) }
    var running by remember { mutableStateOf(true) }
    var job by remember { mutableStateOf<Job?>(null) }

    LaunchedEffect(task, context) {
        job?.cancel()
        answer = ""
        ragHint = ""
        error = null
        running = true
        job = scope.launch {
            try {
                val text = consumeAiAssistStream(task, context) { hint -> ragHint = hint }
                answer = text
                if (text.isBlank()) error = "AI 未返回内容"
            } catch (e: Exception) {
                error = e.message ?: "请求失败"
            } finally {
                running = false
            }
        }
    }

    ModalBottomSheet(onDismissRequest = {
        job?.cancel()
        onDismiss()
    }, containerColor = MaterialTheme.colorScheme.surface) {
        Column(
            Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp)
                .padding(bottom = 28.dp)
                .heightIn(min = 180.dp, max = 520.dp)
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Icon(Icons.Default.AutoAwesome, contentDescription = null, tint = MaterialTheme.colorScheme.primary)
                Text(title, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold)
            }
            if (ragHint.isNotBlank()) {
                Text(ragHint, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            when {
                running && answer.isBlank() -> {
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                        CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
                        Text("AI 正在分析…", color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                }
                error != null && answer.isBlank() -> Text(error ?: "", color = MaterialTheme.colorScheme.error)
                else -> Text(answer, fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurface, lineHeight = 20.sp)
            }
            if (running) {
                TextButton(onClick = { job?.cancel(); running = false; onDismiss() }, modifier = Modifier.align(Alignment.End)) {
                    Text("停止")
                }
            } else {
                TextButton(onClick = onDismiss, modifier = Modifier.align(Alignment.End)) {
                    Text("关闭")
                }
            }
        }
    }
}

private suspend fun consumeAiAssistStream(
    task: String,
    context: String,
    onMeta: (String) -> Unit
): String = withContext(Dispatchers.IO) {
    ApiClient.aiApi.aiAssist(AiAssistRequest(task = task, context = context)).use { body ->
        val source = body.source()
        val answer = StringBuilder()
        var streamError: String? = null
        while (true) {
            val line = source.readUtf8Line() ?: break
            if (!line.startsWith("data:")) continue
            val payload = line.removePrefix("data:").trim()
            if (payload.isBlank() || payload == "[DONE]") continue
            try {
                val json = JSONObject(payload)
                when {
                    json.has("error") -> streamError = json.optString("error")
                    json.has("meta") -> {
                        val meta = json.optJSONObject("meta") ?: continue
                        val tip = meta.optString("degraded_tip")
                        if (tip.isNotBlank()) onMeta(tip)
                        else {
                            val mem = meta.optInt("memory_hits", 0)
                            val sk = meta.optInt("skill_hits", 0)
                            if (mem > 0 || sk > 0) {
                                val parts = mutableListOf<String>()
                                if (mem > 0) parts += "记忆 ×$mem"
                                if (sk > 0) parts += "技能 ×$sk"
                                onMeta(parts.joinToString(" · "))
                            }
                        }
                    }
                    json.has("delta") -> answer.append(json.optString("delta"))
                    json.has("reply") -> answer.append(json.optString("reply"))
                    json.has("content") -> answer.append(json.optString("content"))
                }
            } catch (_: Exception) { /* skip */ }
        }
        streamError?.let { throw IOException(it) }
        answer.toString().trim()
    }
}
