package com.aiops.monitor.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.navigation.NavHostController

private val InfraBlue = Color(0xFF4F7FFF)

private data class InfraTab(val label: String, val subtitle: String)

private val infraTabs = listOf(
    InfraTab("主机", "Agent 指标 · 资源利用率"),
    InfraTab("资源", "物理机 · 虚拟机"),
    InfraTab("网络", "网络设备 · 流量 · Trap · 内容审计"),
    InfraTab("拨测", "拨测 · API · 证书"),
)

/**
 * 基础设施监控中枢：把原先分散的 主机(Agent 指标) / 硬件(Redfish) / 网络流量 / 拨测证书
 * 收进一个带子标签的页面，统一入口、命名清晰。硬件与虚拟机合并为「资源」Tab。
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun InfraHubScreen(navController: NavHostController, modifier: Modifier = Modifier, initialTab: Int = 0, initialResSeg: Int = 0, showBack: Boolean = false) {
    var tab by remember { mutableIntStateOf(initialTab.coerceIn(0, infraTabs.lastIndex)) }
    var refreshTick by remember { mutableIntStateOf(0) }

    Scaffold(
        modifier = modifier,
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            Column {
                TopAppBar(
                    title = {
                        Column {
                            Text("基础设施", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface)
                            Spacer(Modifier.height(2.dp))
                            Text(infraTabs[tab].subtitle, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    },
                    navigationIcon = {
                        if (showBack) IconButton(onClick = { navController.popBackStack() }) {
                            Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "返回", tint = MaterialTheme.colorScheme.onSurface)
                        }
                    },
                    actions = {
                        IconButton(onClick = { refreshTick++ }) {
                            Icon(Icons.Default.Refresh, contentDescription = "刷新", tint = MaterialTheme.colorScheme.onSurface)
                        }
                    },
                    colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background)
                )
                ScrollableTabRow(
                    selectedTabIndex = tab,
                    containerColor = MaterialTheme.colorScheme.background,
                    contentColor = InfraBlue,
                    edgePadding = 12.dp,
                    divider = {}
                ) {
                    infraTabs.forEachIndexed { i, t ->
                        Tab(
                            selected = tab == i,
                            onClick = { tab = i },
                            text = { Text(t.label, fontWeight = if (tab == i) FontWeight.Bold else FontWeight.Normal, fontSize = 14.sp) }
                        )
                    }
                }
            }
        }
    ) { padding ->
        Box(Modifier.fillMaxSize().padding(padding)) {
            when (tab) {
                0 -> HostListContent(navController, refreshSignal = refreshTick)
                1 -> ResourceContent(navController, refreshSignal = refreshTick, initialSubTab = initialResSeg)
                2 -> NetworkContent(refreshSignal = refreshTick)
                else -> ProbeContent(refreshSignal = refreshTick)
            }
        }
    }
}

/**
 * 合并后的「资源」Tab：二级 Tab 切换「物理机」和「虚拟机」。
 * 各子标签内部保留各自的搜索框，此处不再提供统一搜索栏以避免重复。
 */
@Composable
private fun ResourceContent(navController: NavHostController, refreshSignal: Int = 0, initialSubTab: Int = 0) {
    var subTab by remember { mutableIntStateOf(initialSubTab.coerceIn(0, 1)) }

    Column(Modifier.fillMaxSize().background(MaterialTheme.colorScheme.background)) {
        // 二级 Tab
        ScrollableTabRow(
            selectedTabIndex = subTab,
            containerColor = MaterialTheme.colorScheme.background,
            contentColor = InfraBlue,
            edgePadding = 12.dp,
            divider = {}
        ) {
            listOf("物理机", "虚拟机").forEachIndexed { i, label ->
                Tab(
                    selected = subTab == i,
                    onClick = { subTab = i },
                    text = { Text(label, fontWeight = if (subTab == i) FontWeight.Bold else FontWeight.Normal, fontSize = 13.sp) }
                )
            }
        }

        Box(Modifier.weight(1f).fillMaxWidth()) {
            when (subTab) {
                0 -> HardwareContent(refreshSignal = refreshSignal)
                1 -> HyperVContent(navController, refreshSignal = refreshSignal)
            }
        }
    }
}