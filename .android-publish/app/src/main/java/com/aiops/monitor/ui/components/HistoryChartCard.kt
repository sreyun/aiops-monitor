package com.aiops.monitor.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Fullscreen
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

/**
 * 统一历史曲线卡片：标题 + 满屏放大 + 足够高度，避免内容挤压。
 * 拨测/API/NetFlow/SNMP 等历史弹窗共用此组件。
 */
@Composable
fun HistoryChartCard(
    title: String,
    values: List<Double>,
    timestamps: List<Long>,
    unit: String,
    color: Color,
    onExpand: () -> Unit,
    modifier: Modifier = Modifier,
    chartHeight: Dp = 300.dp,
    pointDetails: List<String> = emptyList(),
) {
    Card(
        modifier.fillMaxWidth(),
        shape = RoundedCornerShape(12.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 1.dp)
    ) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    title,
                    fontWeight = FontWeight.Bold,
                    fontSize = 13.sp,
                    color = MaterialTheme.colorScheme.onSurface,
                    modifier = Modifier.weight(1f)
                )
                IconButton(onClick = onExpand, modifier = Modifier.size(30.dp)) {
                    Icon(Icons.Default.Fullscreen, "满屏放大查看", tint = color, modifier = Modifier.size(19.dp))
                }
            }
            InteractiveTimeSeriesChart(
                values = values,
                timestamps = timestamps,
                color = color,
                unit = unit,
                pointDetails = pointDetails,
                modifier = Modifier.fillMaxWidth().height(chartHeight)
            )
        }
    }
}
