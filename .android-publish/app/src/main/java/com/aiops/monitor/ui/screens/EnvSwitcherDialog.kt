package com.aiops.monitor.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.DeleteOutline
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.RadioButtonUnchecked
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import com.aiops.monitor.data.store.ServerEntry

/**
 * 登录页右下角隐藏入口唤出的环境/服务器管理面板。
 * 作为地址切换的唯一入口：支持添加、更名、编辑、删除、切换，并标识当前地址。
 */
@Composable
fun EnvSwitcherDialog(
    servers: List<ServerEntry>,
    currentUrl: String?,
    error: String?,
    onSwitch: (ServerEntry) -> Unit,
    onSave: (url: String, label: String, editingUrl: String?) -> Unit,
    onDelete: (ServerEntry) -> Unit,
    onDismiss: () -> Unit,
) {
    var editing by remember { mutableStateOf<ServerEntry?>(null) }
    var showForm by remember { mutableStateOf(false) }
    var formUrl by remember { mutableStateOf("") }
    var formLabel by remember { mutableStateOf("") }
    var deleteTarget by remember { mutableStateOf<ServerEntry?>(null) }

    fun openAdd() {
        editing = null
        formUrl = ""
        formLabel = ""
        showForm = true
    }
    fun openEdit(entry: ServerEntry) {
        editing = entry
        formUrl = entry.url
        formLabel = entry.label
        showForm = true
    }

    Dialog(onDismissRequest = onDismiss) {
        Card(
            shape = RoundedCornerShape(18.dp),
            colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
            modifier = Modifier.fillMaxWidth().heightIn(max = 560.dp),
        ) {
            Column(
                Modifier.fillMaxWidth().padding(18.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        "环境与服务器",
                        style = MaterialTheme.typography.titleMedium,
                        fontWeight = FontWeight.Bold,
                        color = MaterialTheme.colorScheme.onSurface,
                        modifier = Modifier.weight(1f),
                    )
                    TextButton(onClick = { openAdd() }) {
                        Icon(Icons.Default.Add, null, Modifier.size(16.dp))
                        Spacer(Modifier.width(4.dp))
                        Text("添加", fontSize = 13.sp)
                    }
                }

                Text(
                    "连点右下角唤出。切换后需重新登录。",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )

                error?.let {
                    Text(it, color = Color(0xFFEF4444), style = MaterialTheme.typography.bodySmall)
                }

                if (showForm) {
                    EnvServerForm(
                        title = if (editing == null) "添加服务器" else "编辑服务器",
                        url = formUrl,
                        label = formLabel,
                        onUrlChange = { formUrl = it },
                        onLabelChange = { formLabel = it },
                        onCancel = { showForm = false },
                        onConfirm = {
                            onSave(formUrl.trim(), formLabel.trim(), editing?.url)
                            showForm = false
                        },
                    )
                }

                Column(
                    Modifier
                        .weight(1f, fill = false)
                        .fillMaxWidth()
                        .verticalScroll(rememberScrollState()),
                    verticalArrangement = Arrangement.spacedBy(0.dp),
                ) {
                    if (servers.isEmpty() && !showForm) {
                        Box(
                            Modifier.fillMaxWidth().padding(vertical = 28.dp),
                            contentAlignment = Alignment.Center,
                        ) {
                            Text(
                                "暂无服务器，请先添加访问地址",
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                    servers.forEachIndexed { index, server ->
                        val isActive = normalizeUrlKey(server.url) == normalizeUrlKey(currentUrl)
                        EnvServerRow(
                            server = server,
                            isActive = isActive,
                            onSwitch = { onSwitch(server) },
                            onEdit = { openEdit(server) },
                            onDelete = { deleteTarget = server },
                        )
                        if (index < servers.lastIndex) {
                            HorizontalDivider(
                                color = MaterialTheme.colorScheme.outlineVariant,
                                thickness = 0.5.dp,
                            )
                        }
                    }
                }

                OutlinedButton(
                    onClick = onDismiss,
                    modifier = Modifier.fillMaxWidth(),
                    shape = RoundedCornerShape(10.dp),
                ) { Text("关闭", fontSize = 13.sp) }
            }
        }
    }

    deleteTarget?.let { target ->
        AlertDialog(
            onDismissRequest = { deleteTarget = null },
            containerColor = MaterialTheme.colorScheme.surface,
            shape = RoundedCornerShape(16.dp),
            title = { Text("确认删除", fontSize = 16.sp, fontWeight = FontWeight.Bold) },
            text = {
                Text(
                    "确定删除「${target.label.ifBlank { target.url }}」吗？",
                    fontSize = 13.sp,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            },
            confirmButton = {
                Button(
                    onClick = {
                        onDelete(target)
                        deleteTarget = null
                    },
                    colors = ButtonDefaults.buttonColors(containerColor = Color(0xFFEF4444)),
                    shape = RoundedCornerShape(10.dp),
                ) { Text("删除", fontSize = 13.sp) }
            },
            dismissButton = {
                TextButton(onClick = { deleteTarget = null }) { Text("取消", fontSize = 13.sp) }
            },
        )
    }
}

@Composable
private fun EnvServerForm(
    title: String,
    url: String,
    label: String,
    onUrlChange: (String) -> Unit,
    onLabelChange: (String) -> Unit,
    onCancel: () -> Unit,
    onConfirm: () -> Unit,
) {
    Column(
        Modifier
            .fillMaxWidth()
            .background(MaterialTheme.colorScheme.background, RoundedCornerShape(12.dp))
            .padding(12.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Text(title, fontWeight = FontWeight.Bold, fontSize = 14.sp, color = Color(0xFF4F7FFF))
        OutlinedTextField(
            value = label,
            onValueChange = onLabelChange,
            modifier = Modifier.fillMaxWidth(),
            label = { Text("名称（如：正式库 / 测试库）") },
            singleLine = true,
            shape = RoundedCornerShape(12.dp),
            colors = loginFieldColors(),
        )
        OutlinedTextField(
            value = url,
            onValueChange = onUrlChange,
            modifier = Modifier.fillMaxWidth(),
            label = { Text("服务器地址") },
            placeholder = { Text("http:// 或 https://") },
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
            shape = RoundedCornerShape(12.dp),
            colors = loginFieldColors(),
        )
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedButton(onClick = onCancel, modifier = Modifier.weight(1f), shape = RoundedCornerShape(10.dp)) {
                Text("取消", fontSize = 13.sp)
            }
            Button(
                onClick = onConfirm,
                modifier = Modifier.weight(1f),
                enabled = url.isNotBlank(),
                shape = RoundedCornerShape(10.dp),
            ) { Text("保存", fontSize = 13.sp) }
        }
    }
}

@Composable
private fun EnvServerRow(
    server: ServerEntry,
    isActive: Boolean,
    onSwitch: () -> Unit,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .background(if (isActive) Color(0xFF4F7FFF).copy(alpha = 0.08f) else Color.Transparent)
            .padding(vertical = 10.dp, horizontal = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            if (isActive) Icons.Default.CheckCircle else Icons.Default.RadioButtonUnchecked,
            contentDescription = null,
            tint = if (isActive) Color(0xFF00A86B) else MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.size(20.dp),
        )
        Spacer(Modifier.width(10.dp))
        Column(Modifier.weight(1f)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    server.label.ifBlank { "未命名" },
                    style = MaterialTheme.typography.bodyMedium,
                    fontWeight = FontWeight.Bold,
                    color = if (isActive) Color(0xFF00A86B) else MaterialTheme.colorScheme.onSurface,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f, fill = false),
                )
                if (isActive) {
                    Spacer(Modifier.width(6.dp))
                    Text(
                        "当前",
                        fontSize = 10.sp,
                        fontWeight = FontWeight.Bold,
                        color = Color(0xFF00A86B),
                        modifier = Modifier
                            .background(Color(0xFF00A86B).copy(alpha = 0.12f), RoundedCornerShape(4.dp))
                            .padding(horizontal = 6.dp, vertical = 2.dp),
                    )
                }
            }
            Text(
                server.url,
                style = MaterialTheme.typography.bodySmall,
                color = if (isActive) Color(0xFF4F7FFF) else MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
        if (!isActive) {
            TextButton(onClick = onSwitch) {
                Text("切换", color = Color(0xFF4F7FFF), fontWeight = FontWeight.Bold, fontSize = 12.sp)
            }
        }
        IconButton(onClick = onEdit, modifier = Modifier.size(32.dp)) {
            Icon(Icons.Default.Edit, contentDescription = "编辑", tint = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.size(18.dp))
        }
        IconButton(onClick = onDelete, modifier = Modifier.size(32.dp)) {
            Icon(Icons.Default.DeleteOutline, contentDescription = "删除", tint = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.size(18.dp))
        }
    }
}

private fun normalizeUrlKey(url: String?): String =
    (url ?: "").trim().trimEnd('/').lowercase()
