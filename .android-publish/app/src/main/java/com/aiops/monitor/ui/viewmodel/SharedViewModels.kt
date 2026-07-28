package com.aiops.monitor.ui.viewmodel

import androidx.activity.ComponentActivity
import androidx.compose.runtime.Composable
import androidx.compose.ui.platform.LocalContext
import androidx.lifecycle.viewmodel.compose.viewModel

/**
 * Activity 作用域的 AI 助手 VM。
 *
 * AlertsScreen 的“AI 诊断”按钮要把告警上下文交给 AiCopilotScreen 里的**同一个**
 * VM 实例消费（setPendingAlertDiagnosis → processPendingAlert）。若两处各用
 * route 级 viewModel()，就是两个不同实例，桥接不上。这里统一挂到 Activity 上，
 * 顺带让 AI 对话历史在 App 会话内保持连续。
 */
@Composable
fun aiCopilotViewModel(): AiCopilotViewModel {
    val owner = LocalContext.current as ComponentActivity
    return viewModel(viewModelStoreOwner = owner)
}
