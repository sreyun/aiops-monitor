package com.aiops.monitor.ui.screens

import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Group
import androidx.compose.material.icons.filled.History
import androidx.compose.material.icons.filled.Info
import androidx.compose.material.icons.filled.OpenInNew
import androidx.compose.material.icons.filled.Password
import androidx.compose.material.icons.filled.Payments
import androidx.compose.material.icons.filled.Terminal
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.navigation.NavHostController
import com.aiops.monitor.data.ApiClient
import com.aiops.monitor.data.store.SettingsStore
import com.aiops.monitor.ui.Routes

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(
    settingsStore: SettingsStore,
    navController: NavHostController,
    modifier: Modifier = Modifier,
    initialError: String? = null
) {
    val context = LocalContext.current

    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Text(
                        "配置中心",
                        fontWeight = FontWeight.Bold,
                        style = MaterialTheme.typography.titleMedium,
                        color = MaterialTheme.colorScheme.onSurface
                    )
                },
                navigationIcon = {
                    IconButton(onClick = { navController.popBackStack() }) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "返回", tint = MaterialTheme.colorScheme.onSurface)
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.background,
                    titleContentColor = MaterialTheme.colorScheme.onSurface
                )
            )
        },
        containerColor = MaterialTheme.colorScheme.background
    ) { padding ->
        Column(
            modifier = modifier
                .fillMaxSize()
                .padding(padding)
                .imePadding()
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp)
        ) {
            initialError?.let {
                Text(it, color = Color(0xFFEF4444), style = MaterialTheme.typography.bodySmall)
            }

            // 安装 Agent
            SettingsActionCard(
                title = "安装 Agent",
                subtitle = "查看各平台一键安装 / 卸载命令，支持复制",
                icon = { Icon(Icons.Default.Terminal, null, tint = Color(0xFF4F7FFF)) },
                onClick = { navController.navigate(Routes.INSTALL_AGENT) },
                enabled = ApiClient.cookieHeader().isNotBlank(),
            )

            // 成本管理（AI 调用 / Token / 费用，与 Web 观测同步）
            SettingsActionCard(
                title = "成本管理",
                subtitle = if (ApiClient.cookieHeader().isBlank()) "登录后可查看 AI 调用与费用"
                else "调用次数 · 延迟 · Token · 费用趋势",
                icon = { Icon(Icons.Default.Payments, null, tint = Color(0xFFF97316)) },
                onClick = { navController.navigate(Routes.COST_MANAGEMENT) },
                enabled = ApiClient.cookieHeader().isNotBlank(),
            )

            SettingsActionCard(
                title = "用户管理",
                subtitle = if (ApiClient.cookieHeader().isBlank()) "登录后可用（需管理员）"
                else "创建 / 改角色 / 删除用户",
                icon = { Icon(Icons.Default.Group, null, tint = Color(0xFF6366F1)) },
                onClick = { navController.navigate(Routes.USERS_ADMIN) },
                enabled = ApiClient.cookieHeader().isNotBlank(),
            )

            SettingsActionCard(
                title = "活动审计",
                subtitle = if (ApiClient.cookieHeader().isBlank()) "登录后可查看"
                else "操作 / 系统 / 终端活动日志",
                icon = { Icon(Icons.Default.History, null, tint = Color(0xFF0EA5E9)) },
                onClick = { navController.navigate(Routes.ACTIVITY_AUDIT) },
                enabled = ApiClient.cookieHeader().isNotBlank(),
            )

            // 说明：AI 设置 / 告警通道 / Web 备份还原仅管理员可在网页端配置；App 侧相关写操作已按角色限制
            Text(
                "系统级设置（AI Provider、告警通道、备份还原）仅管理员可在网页端修改；成本管理数据为只读观测。用户管理需 admin。",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )

            // 终端二次密码
            SettingsActionCard(
                title = "终端二次密码",
                subtitle = if (ApiClient.cookieHeader().isBlank()) "登录后可设置" else "设置或修改终端二次密码",
                icon = { Icon(Icons.Default.Password, null, tint = Color(0xFF4F7FFF)) },
                onClick = { navController.navigate(Routes.TERMINAL_PASSWORD) },
                enabled = ApiClient.cookieHeader().isNotBlank(),
            )

            // 关于我们
            Card(
                shape = RoundedCornerShape(16.dp),
                colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
                elevation = CardDefaults.cardElevation(defaultElevation = 0.dp)
            ) {
                Column(
                    Modifier.padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(10.dp)
                ) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Icon(Icons.Default.Info, null, tint = Color(0xFF4F7FFF), modifier = Modifier.size(20.dp))
                        Spacer(Modifier.width(8.dp))
                        Text(
                            "关于我们",
                            style = MaterialTheme.typography.titleSmall,
                            fontWeight = FontWeight.Bold,
                            color = Color(0xFF4F7FFF),
                        )
                    }
                    Text(
                        "智能运维（AIOps）是面向企业基础设施的一体化智能运维平台，" +
                            "覆盖主机监控、网络流量、告警治理、自动化巡检与 AI 辅助诊断，" +
                            "帮助运维团队更快发现、定位并处理故障。",
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        lineHeight = 20.sp,
                    )
                    TextButton(
                        onClick = {
                            context.startActivity(
                                Intent(Intent.ACTION_VIEW, Uri.parse("https://aiops.sreyun.com"))
                            )
                        },
                        contentPadding = PaddingValues(0.dp)
                    ) {
                        Icon(Icons.Default.OpenInNew, null, Modifier.size(16.dp))
                        Spacer(Modifier.width(6.dp))
                        Text("访问官网 https://aiops.sreyun.com", fontSize = 13.sp, color = Color(0xFF4F7FFF))
                    }
                }
            }

            Text(
                "服务器访问地址请在登录页右下角连点三次进行管理与切换。",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 4.dp)
            )
        }
    }
}

@Composable
private fun SettingsActionCard(
    title: String,
    subtitle: String,
    icon: @Composable () -> Unit,
    onClick: () -> Unit,
    enabled: Boolean = true,
) {
    Card(
        onClick = onClick,
        enabled = enabled,
        shape = RoundedCornerShape(16.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 0.dp)
    ) {
        Row(
            Modifier.fillMaxWidth().padding(16.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            icon()
            Spacer(Modifier.width(12.dp))
            Column(Modifier.weight(1f)) {
                Text(title, fontWeight = FontWeight.Bold, style = MaterialTheme.typography.titleSmall)
                Text(subtitle, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
    }
}
