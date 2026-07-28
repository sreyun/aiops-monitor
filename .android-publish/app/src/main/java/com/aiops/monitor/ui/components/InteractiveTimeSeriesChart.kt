package com.aiops.monitor.ui.components

import android.graphics.Paint
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CenterFocusStrong
import androidx.compose.material.icons.filled.ZoomIn
import androidx.compose.material.icons.filled.ZoomOut
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableDoubleStateOf
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.nativeCanvas
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.input.pointer.PointerEventType
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import kotlin.math.abs
import kotlin.math.hypot
import kotlin.math.max
import kotlin.math.sqrt

private data class NativeChartPoint(val timestamp: Long, val value: Double, val detail: String)

/** Native Compose time-series explorer with timestamp-based positioning and point inspection. */
@Composable
fun InteractiveTimeSeriesChart(
    values: List<Double>,
    timestamps: List<Long>,
    color: Color,
    unit: String,
    modifier: Modifier = Modifier,
    pointDetails: List<String> = emptyList()
) {
    val scheme = MaterialTheme.colorScheme
    val points = remember(values, timestamps, pointDetails) {
        values.mapIndexedNotNull { index, value ->
            if (!value.isFinite()) return@mapIndexedNotNull null
            val rawTimestamp = timestamps.getOrNull(index) ?: index.toLong()
            val timestamp = normalizeChartTimestamp(rawTimestamp)
            NativeChartPoint(timestamp, value, pointDetails.getOrNull(index).orEmpty())
        }.sortedBy { it.timestamp }
    }
    if (points.size < 2) {
        Box(modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            Text("采样点不足，暂时无法绘制趋势", color = scheme.onSurfaceVariant)
        }
        return
    }

    val fullStart = points.first().timestamp.toDouble()
    val fullEnd = max(points.last().timestamp.toDouble(), fullStart + 1.0)
    val fullSpan = fullEnd - fullStart
    var viewportStart by remember(points) { mutableDoubleStateOf(fullStart) }
    var viewportEnd by remember(points) { mutableDoubleStateOf(fullEnd) }
    var selectedIndex by remember(points) { mutableIntStateOf(points.lastIndex) }
    var canvasWidth by remember { mutableFloatStateOf(1f) }

    fun setViewport(center: Double, span: Double) {
        val minimumSpan = max(fullSpan / 100.0, 1_000.0)
        val targetSpan = span.coerceIn(minimumSpan.coerceAtMost(fullSpan), fullSpan)
        var start = center - targetSpan / 2.0
        var end = center + targetSpan / 2.0
        if (start < fullStart) { end += fullStart - start; start = fullStart }
        if (end > fullEnd) { start -= end - fullEnd; end = fullEnd }
        viewportStart = start.coerceAtLeast(fullStart)
        viewportEnd = end.coerceAtMost(fullEnd)
    }

    val selected = points[selectedIndex.coerceIn(points.indices)]
    Column(modifier, verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Card(
            colors = CardDefaults.cardColors(containerColor = color.copy(alpha = 0.10f)),
            shape = RoundedCornerShape(10.dp)
        ) {
            Column(Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 9.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(formatChartTimestamp(selected.timestamp), color = scheme.onSurface, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
                    Text(formatChartValue(selected.value, unit), color = color, fontWeight = FontWeight.Black, fontSize = 18.sp)
                }
                if (selected.detail.isNotBlank()) {
                    Text(selected.detail, color = scheme.onSurfaceVariant, fontSize = 11.sp, maxLines = 2)
                }
            }
        }

        Box(
            Modifier.weight(1f).fillMaxWidth().background(scheme.surfaceVariant, RoundedCornerShape(12.dp))
        ) {
            Canvas(
                Modifier
                    .fillMaxSize()
                    .onSizeChanged { canvasWidth = it.width.toFloat() }
                    .semantics { contentDescription = "交互式时间趋势图，点按可查看准确时间点，拖动平移，双指缩放" }
                    .pointerInput(points, viewportStart, viewportEnd) {
                        val touchSlopPx = viewConfiguration.touchSlop
                        awaitEachGesture {
                            val down = awaitFirstDown()
                            val downPos = down.position
                            var currentPos = downPos
                            var isMultiTouch = false

                            // 单指阶段：检测点击 / 拖拽平移
                            while (true) {
                                val event = awaitPointerEvent()

                                // ★ 先检查 Release，再检查 pressed（Release 时 pressed=false 会空列表）
                                if (event.type == PointerEventType.Release) {
                                    val moveDist = hypot(
                                        (event.changes.first().position.x - downPos.x).toDouble(),
                                        (event.changes.first().position.y - downPos.y).toDouble()
                                    ).toFloat()
                                    if (moveDist < touchSlopPx) {
                                        // 点击：选中最近数据点
                                        val plotLeft = 54.dp.toPx()
                                        val plotRight = size.width - 12.dp.toPx()
                                        val cx = event.changes.first().position.x
                                        if (cx in plotLeft..plotRight) {
                                            val ratio = ((cx - plotLeft) / (plotRight - plotLeft)).coerceIn(0f, 1f)
                                            val target = viewportStart + ratio * (viewportEnd - viewportStart)
                                            selectedIndex = points.indices.minByOrNull { abs(points[it].timestamp - target) } ?: selectedIndex
                                        }
                                    }
                                    break
                                }

                                val pressed = event.changes.filter { it.pressed }
                                if (pressed.isEmpty()) break

                                if (pressed.size >= 2) {
                                    isMultiTouch = true
                                    break // 进入多指缩放阶段
                                }

                                val change = pressed.first()
                                val dx = change.position.x - currentPos.x
                                val dy = change.position.y - currentPos.y
                                val moveDistance = sqrt(dx * dx + dy * dy)

                                if (moveDistance >= touchSlopPx) {
                                    val currentSpan = viewportEnd - viewportStart
                                    val shiftedCenter = (viewportStart + viewportEnd) / 2.0 -
                                        (dx / canvasWidth.coerceAtLeast(1f)) * currentSpan
                                    setViewport(shiftedCenter, currentSpan)
                                    currentPos = change.position
                                    change.consume()
                                }
                            }

                            // 多指缩放阶段：双指捏合缩放
                            if (isMultiTouch) {
                                var prevEvent = awaitPointerEvent()
                                var prevPressed = prevEvent.changes.filter { it.pressed }
                                if (prevPressed.size >= 2) {
                                    val prevDist = hypot(
                                        (prevPressed[0].position.x - prevPressed[1].position.x).toDouble(),
                                        (prevPressed[0].position.y - prevPressed[1].position.y).toDouble()
                                    )
                                    var lastDist = prevDist

                                    while (true) {
                                        val ev = awaitPointerEvent()
                                        val p = ev.changes.filter { it.pressed }
                                        if (p.size < 2) break

                                        val newDist = hypot(
                                            (p[0].position.x - p[1].position.x).toDouble(),
                                            (p[0].position.y - p[1].position.y).toDouble()
                                        )
                                        if (lastDist > 0) {
                                            val zoomFactor = (newDist / lastDist).toFloat()
                                            val midX = (p[0].position.x + p[1].position.x) / 2f
                                            val panDx = midX - (prevPressed[0].position.x + prevPressed[1].position.x) / 2f
                                            val currentSpan = viewportEnd - viewportStart
                                            val shiftedCenter = (viewportStart + viewportEnd) / 2.0 -
                                                (panDx / canvasWidth.coerceAtLeast(1f)) * currentSpan
                                            setViewport(shiftedCenter, currentSpan / zoomFactor)
                                        }
                                        lastDist = newDist
                                        prevPressed = p
                                        p.forEach { it.consume() }
                                    }
                                }
                            }
                        }
                    }
            ) {
                val left = 54.dp.toPx()
                val right = size.width - 12.dp.toPx()
                val top = 14.dp.toPx()
                val bottom = size.height - 32.dp.toPx()
                if (right <= left || bottom <= top) return@Canvas
                val visible = points.filter { it.timestamp.toDouble() in viewportStart..viewportEnd }
                if (visible.isEmpty()) return@Canvas
                var minValue = visible.minOf { it.value }
                var maxValue = visible.maxOf { it.value }
                if (abs(maxValue - minValue) < 0.0001) { minValue -= 1.0; maxValue += 1.0 }
                val padding = (maxValue - minValue) * 0.12
                minValue -= padding
                maxValue += padding
                val valueSpan = maxValue - minValue
                val timeSpan = (viewportEnd - viewportStart).coerceAtLeast(1.0)

                fun x(ts: Long) = left + (((ts - viewportStart) / timeSpan).toFloat() * (right - left))
                fun y(value: Double) = bottom - (((value - minValue) / valueSpan).toFloat() * (bottom - top))

                repeat(5) { index ->
                    val gy = top + (bottom - top) * index / 4f
                    drawLine(scheme.outlineVariant, Offset(left, gy), Offset(right, gy), 1.dp.toPx())
                }
                repeat(4) { index ->
                    val gx = left + (right - left) * index / 3f
                    drawLine(scheme.outlineVariant.copy(alpha = 0.72f), Offset(gx, top), Offset(gx, bottom), 1.dp.toPx())
                }

                val path = Path()
                visible.forEachIndexed { index, point ->
                    val p = Offset(x(point.timestamp), y(point.value))
                    if (index == 0) path.moveTo(p.x, p.y) else path.lineTo(p.x, p.y)
                }
                drawPath(path, color, style = Stroke(width = 2.2.dp.toPx(), cap = StrokeCap.Round))
                if (visible.size <= 80) {
                    visible.forEach { drawCircle(color, 2.2.dp.toPx(), Offset(x(it.timestamp), y(it.value))) }
                }

                if (selected.timestamp.toDouble() in viewportStart..viewportEnd) {
                    val sx = x(selected.timestamp)
                    val sy = y(selected.value)
                    drawLine(color.copy(alpha = 0.65f), Offset(sx, top), Offset(sx, bottom), 1.dp.toPx())
                    drawLine(color.copy(alpha = 0.35f), Offset(left, sy), Offset(right, sy), 1.dp.toPx())
                    drawCircle(scheme.surfaceVariant, 6.dp.toPx(), Offset(sx, sy))
                    drawCircle(color, 4.dp.toPx(), Offset(sx, sy))
                }

                val labelPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
                    textSize = 10.sp.toPx()
                    this.color = scheme.onSurfaceVariant.toArgb()
                }
                repeat(5) { index ->
                    val value = maxValue - valueSpan * index / 4.0
                    val gy = top + (bottom - top) * index / 4f
                    drawContext.canvas.nativeCanvas.drawText(compactChartValue(value, unit), 2.dp.toPx(), gy + 4.dp.toPx(), labelPaint)
                }
                labelPaint.textAlign = Paint.Align.CENTER
                repeat(3) { index ->
                    val ts = (viewportStart + (viewportEnd - viewportStart) * index / 2.0).toLong()
                    val gx = left + (right - left) * index / 2f
                    drawContext.canvas.nativeCanvas.drawText(formatChartAxisTime(ts, fullSpan), gx, size.height - 8.dp.toPx(), labelPaint)
                }
            }
        }

        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Text("点按定位 · 拖动平移 · 双指缩放", color = scheme.onSurfaceVariant, fontSize = 10.sp, modifier = Modifier.weight(1f))
            IconButton(onClick = { setViewport((viewportStart + viewportEnd) / 2.0, (viewportEnd - viewportStart) * 1.8) }) {
                Icon(Icons.Default.ZoomOut, "缩小时间范围", tint = scheme.onSurfaceVariant, modifier = Modifier.size(19.dp))
            }
            IconButton(onClick = { setViewport((viewportStart + viewportEnd) / 2.0, (viewportEnd - viewportStart) / 1.8) }) {
                Icon(Icons.Default.ZoomIn, "放大时间范围", tint = color, modifier = Modifier.size(19.dp))
            }
            IconButton(onClick = { viewportStart = fullStart; viewportEnd = fullEnd; selectedIndex = points.lastIndex }) {
                Icon(Icons.Default.CenterFocusStrong, "重置图表", tint = Color(0xFF9AA4B8), modifier = Modifier.size(19.dp))
            }
        }
    }
}

internal fun normalizeChartTimestamp(timestamp: Long): Long =
    if (timestamp in 1..999_999_999_999L) timestamp * 1_000L else timestamp

private fun formatChartValue(value: Double, unit: String): String {
    val u = unit.trim()
    if (u == "%") return "%.1f%%".format(value)
    val (scaled, suffix) = scaleChartValue(value, u)
    val num = when {
        abs(scaled) >= 100 -> "%.0f".format(scaled)
        abs(scaled) >= 10 -> "%.1f".format(scaled)
        else -> "%.2f".format(scaled)
    }
    return if (suffix.isBlank()) num else "$num $suffix"
}

/** 轴标签：紧凑显示，只保留量级前缀（如 1.5M），避免窄轴溢出。 */
private fun compactChartValue(value: Double, unit: String): String {
    val u = unit.trim()
    if (u == "%") return "%.0f".format(value)
    val base = if (u == "B" || u == "bytes" || u == "Bps") 1024.0 else 1000.0
    val prefixes = listOf("", "K", "M", "G", "T")
    var v = value
    var i = 0
    while (abs(v) >= base && i < prefixes.lastIndex) { v /= base; i++ }
    val num = if (abs(v) >= 100) "%.0f".format(v) else "%.1f".format(v)
    return num + prefixes[i]
}

/**
 * 按单位智能缩放数值以增强可读性：
 * bps 按 1000 进制（bps/Kbps/Mbps…），字节按 1024 进制（B/KB/MB…），
 * 其它计数类单位在超过千时用 K/M/G 前缀并保留原单位。
 */
private fun scaleChartValue(value: Double, unit: String): Pair<Double, String> {
    fun steps(base: Double, units: List<String>): Pair<Double, String> {
        var v = value
        var i = 0
        while (abs(v) >= base && i < units.lastIndex) { v /= base; i++ }
        return v to units[i]
    }
    return when (unit) {
        "bps" -> steps(1000.0, listOf("bps", "Kbps", "Mbps", "Gbps", "Tbps"))
        "Bps" -> steps(1024.0, listOf("Bps", "KBps", "MBps", "GBps", "TBps"))
        "B", "bytes" -> steps(1024.0, listOf("B", "KB", "MB", "GB", "TB"))
        "" -> value to ""
        else -> if (abs(value) >= 1000) {
            val (v, p) = steps(1000.0, listOf("", "K", "M", "G", "T"))
            v to (p + unit)
        } else value to unit
    }
}

private fun formatChartTimestamp(timestamp: Long): String =
    SimpleDateFormat("yyyy-MM-dd HH:mm:ss", Locale.getDefault()).format(Date(timestamp))

private fun formatChartAxisTime(timestamp: Long, fullSpan: Double): String =
    SimpleDateFormat(if (fullSpan >= 86_400_000) "MM-dd HH:mm" else "HH:mm:ss", Locale.getDefault()).format(Date(timestamp))
