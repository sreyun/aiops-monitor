package com.aiops.monitor.ui.components

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.Button
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material3.Icon
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.nativeCanvas
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/* ── 全应用统一语义色 ── */
// 所有 Screen 统一引用这些常量，替换各自私有的重复定义
val AppBlue = Color(0xFF4F7FFF)
val AppGreen = Color(0xFF00A86B)
val AppOrange = Color(0xFFF59E0B)
val AppRed = Color(0xFFEF4444)

/* ── 语义色映射 ── */

@Composable
fun statusColor(level: String): Color = when (level.lowercase()) {
    "critical", "error", "down", "offline" -> AppRed
    "warning", "warn" -> AppOrange
    "ok", "success", "up", "online", "info" -> AppGreen
    else -> MaterialTheme.colorScheme.outline
}

@Composable
fun thresholdColor(percent: Float): Color = when {
    percent >= 85f -> AppRed
    percent >= 70f -> AppOrange
    else -> AppBlue
}

/* ── 基础元件 ── */

@Composable
fun StatusDot(color: Color, size: androidx.compose.ui.unit.Dp = 10.dp) {
    Box(
        Modifier
            .size(size)
            .clip(CircleShape)
            .background(color)
    )
}

@Composable
fun StatusPill(text: String, color: Color, modifier: Modifier = Modifier) {
    Box(
        modifier
            .clip(RoundedCornerShape(6.dp))
            .background(color.copy(alpha = 0.15f))
            .padding(horizontal = 8.dp, vertical = 3.dp)
    ) {
        Text(
            text,
            style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.sp, letterSpacing = 0.3.sp),
            color = color,
            fontWeight = FontWeight.Bold
        )
    }
}

/* ── 趋势 Sparkline ── */
@Composable
fun Sparkline(
    data: List<Double>,
    color: Color,
    modifier: Modifier = Modifier,
    timestamps: List<Long> = emptyList(),
    showAxes: Boolean = false,
    showGrid: Boolean = false,
    yLabel: String = "%",
    currentValue: Double? = null
) {
    val scheme = MaterialTheme.colorScheme
    val chartPoints = remember(data, timestamps) {
        data.mapIndexedNotNull { index, value ->
            value.takeIf { it.isFinite() }?.let { Triple(index, it, timestamps.getOrNull(index) ?: 0L) }
        }
    }
    val xAxisLabels = remember(chartPoints) {
        if (chartPoints.size < 2 || chartPoints.none { it.third > 0 }) {
            emptyList()
        } else {
            val validTimes = chartPoints.map { it.third }.filter { it > 0 }
            val span = (validTimes.maxOrNull() ?: 0L) - (validTimes.minOrNull() ?: 0L)
            val format = SimpleDateFormat(if (span >= 86_400) "MM-dd HH:mm" else "HH:mm", Locale.getDefault())
            listOf(0, chartPoints.lastIndex / 2, chartPoints.lastIndex).distinct().map { index ->
                val timestamp = chartPoints[index].third
                index to if (timestamp > 0) {
                    val millis = if (timestamp > 1_000_000_000_000L) timestamp else timestamp * 1000
                    format.format(Date(millis))
                } else ""
            }.filter { it.second.isNotBlank() }
        }
    }
    Canvas(modifier = modifier) {
        val points = chartPoints.map { it.second }
        if (points.size < 2) return@Canvas
        val max = points.maxOrNull() ?: 1.0
        val min = points.minOrNull() ?: 0.0
        val range = (max - min).coerceAtLeast(0.1)

        // 为 Y 轴数值与 X 轴真实采样时间预留空间。
        val axisLeft = if (showAxes) 36.dp.toPx() else 0f
        val axisBottom = if (showAxes) 26.dp.toPx() else 0f
        val axisRight = if (showAxes) 3.dp.toPx() else 0f
        val chartWidth = size.width - axisLeft - axisRight
        val chartHeight = size.height - axisBottom

        if (chartWidth <= 0f || chartHeight <= 0f) return@Canvas

        // ── 网格 ──
        if (showGrid) {
            val gridColor = scheme.outlineVariant
            val dashWidth = 4.dp.toPx()
            val gapWidth = 4.dp.toPx()
            val gridLevels = listOf(0.25, 0.5, 0.75, 1.0)
            for (level in gridLevels) {
                val y = chartHeight * (1f - level.toFloat())
                var x = axisLeft
                while (x < size.width) {
                    drawLine(
                        color = gridColor,
                        start = Offset(x, y),
                        end = Offset((x + dashWidth).coerceAtMost(size.width), y),
                        strokeWidth = 0.5.dp.toPx()
                    )
                    x += dashWidth + gapWidth
                }
            }
            if (showAxes && xAxisLabels.isNotEmpty()) {
                xAxisLabels.forEach { (index, _) ->
                    val x = axisLeft + index * (chartWidth / (points.size - 1))
                    drawLine(
                        color = gridColor,
                        start = Offset(x, 0f),
                        end = Offset(x, chartHeight),
                        strokeWidth = 0.5.dp.toPx()
                    )
                }
            }
        }

        // ── Y 轴标签 ──
        if (showAxes) {
            val paint = android.graphics.Paint().apply {
                this.color = scheme.onSurfaceVariant.toArgb()
                textSize = 9.sp.toPx()
                textAlign = android.graphics.Paint.Align.RIGHT
                isAntiAlias = true
            }
            val yLabels = listOf(0.0, 0.25, 0.5, 0.75, 1.0)
            for (level in yLabels) {
                val y = chartHeight * (1f - level.toFloat())
                val label = "%.0f%s".format(min + range * level, yLabel)
                drawContext.canvas.nativeCanvas.drawText(label, axisLeft - 4.dp.toPx(), y + 3.dp.toPx(), paint)
            }
            // Y 轴线
            drawLine(
                color = scheme.outline,
                start = Offset(axisLeft, 0f),
                end = Offset(axisLeft, chartHeight),
                strokeWidth = 1.dp.toPx()
            )
            // X 轴线
            drawLine(
                color = scheme.outline,
                start = Offset(axisLeft, chartHeight),
                end = Offset(axisLeft + chartWidth, chartHeight),
                strokeWidth = 1.dp.toPx()
            )

            if (xAxisLabels.isNotEmpty()) {
                val timePaint = android.graphics.Paint().apply {
                    this.color = scheme.onSurfaceVariant.toArgb()
                    textSize = 9.sp.toPx()
                    isAntiAlias = true
                }
                xAxisLabels.forEachIndexed { position, (index, label) ->
                    val x = axisLeft + index * (chartWidth / (points.size - 1))
                    timePaint.textAlign = when (position) {
                        0 -> android.graphics.Paint.Align.LEFT
                        xAxisLabels.lastIndex -> android.graphics.Paint.Align.RIGHT
                        else -> android.graphics.Paint.Align.CENTER
                    }
                    drawContext.canvas.nativeCanvas.drawText(
                        label,
                        x,
                        chartHeight + 17.dp.toPx(),
                        timePaint
                    )
                }
            }
        }

        // ── 数据曲线 + 渐变填充 ──
        val linePath = Path()
        val fillPath = Path()
        points.forEachIndexed { index, value ->
            val x = axisLeft + index * (chartWidth / (points.size - 1))
            val y = chartHeight - ((value - min) / range * chartHeight).toFloat()
            if (index == 0) {
                linePath.moveTo(x, y)
                fillPath.moveTo(x, chartHeight)
                fillPath.lineTo(x, y)
            } else {
                linePath.lineTo(x, y)
                fillPath.lineTo(x, y)
            }
        }
        // 封闭填充路径
        fillPath.lineTo(axisLeft + chartWidth, chartHeight)
        fillPath.close()

        // 渐变填充
        drawPath(
            path = fillPath,
            brush = Brush.verticalGradient(
                colors = listOf(color.copy(alpha = 0.25f), color.copy(alpha = 0.02f)),
                startY = 0f,
                endY = chartHeight
            )
        )

        // 曲线
        drawPath(
            path = linePath,
            color = color,
            style = Stroke(width = 2.dp.toPx())
        )

        // ── 当前值标签 ──
        if (showAxes && currentValue != null && currentValue.isFinite()) {
            val lastX = axisLeft + chartWidth
            val lastY = chartHeight - ((currentValue - min) / range * chartHeight).toFloat()
            drawCircle(color = color, radius = 4.dp.toPx(), center = Offset(lastX, lastY))
            drawCircle(color = scheme.surface, radius = 2.dp.toPx(), center = Offset(lastX, lastY))
            val valuePaint = android.graphics.Paint().apply {
                this.color = scheme.onSurface.toArgb()
                textSize = 11.sp.toPx()
                textAlign = android.graphics.Paint.Align.RIGHT
                isAntiAlias = true
                isFakeBoldText = true
            }
            val valueStr = "%.1f%s".format(currentValue, yLabel)
            drawContext.canvas.nativeCanvas.drawText(
                valueStr,
                lastX - 6.dp.toPx(),
                lastY - 6.dp.toPx(),
                valuePaint
            )
        }
    }
}

/* ── 统计卡（与 MonitorScreen SummaryTile 尺寸/风格对齐）── */
@Composable
fun StatCard(
    label: String,
    value: String,
    accent: Color,
    modifier: Modifier = Modifier,
    selected: Boolean = false,
    onClick: (() -> Unit)? = null
) {
    val scheme = MaterialTheme.colorScheme
    Card(
        onClick = { onClick?.invoke() },
        modifier = modifier.border(
            1.dp,
            if (selected) accent.copy(alpha = 0.4f) else accent.copy(alpha = 0.15f),
            RoundedCornerShape(14.dp)
        ),
        shape = RoundedCornerShape(14.dp),
        colors = CardDefaults.cardColors(containerColor = scheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 2.dp)
    ) {
        Column(
            Modifier.fillMaxWidth().padding(vertical = 12.dp),
            horizontalAlignment = Alignment.CenterHorizontally
        ) {
            Text(
                value,
                color = accent,
                fontWeight = FontWeight.Black,
                fontSize = 22.sp,
                maxLines = 1
            )
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    label,
                    style = MaterialTheme.typography.labelSmall,
                    color = scheme.onSurfaceVariant
                )
                if (onClick != null) {
                    Spacer(Modifier.width(3.dp))
                    Icon(
                        Icons.AutoMirrored.Filled.ArrowForward,
                        contentDescription = "查看$label",
                        tint = accent,
                        modifier = Modifier.size(11.dp)
                    )
                }
            }
        }
    }
}

/* ── 指标进度条（增强版：加粗进度条） ── */
@Composable
fun MetricBar(
    label: String,
    valueText: String,
    percent: Float,
    color: Color,
    modifier: Modifier = Modifier,
    detailText: String? = null
) {
    Column(modifier) {
        Row(Modifier.fillMaxWidth(), Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
            Text(
                label,
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                fontSize = 11.sp
            )
            Row(verticalAlignment = Alignment.CenterVertically) {
                if (detailText != null) {
                    Text(
                        detailText,
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        fontSize = 10.sp
                    )
                    Spacer(Modifier.width(6.dp))
                }
                Text(
                    valueText,
                    style = MaterialTheme.typography.labelSmall,
                    fontWeight = FontWeight.Bold,
                    color = color,
                    fontSize = 11.sp
                )
            }
        }
        Spacer(Modifier.height(6.dp))
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .height(8.dp)
                .clip(RoundedCornerShape(4.dp))
                .background(MaterialTheme.colorScheme.outlineVariant)
        ) {
            Box(
                modifier = Modifier
                    .fillMaxWidth(percent.coerceIn(0f, 1f))
                    .fillMaxHeight()
                    .background(
                        Brush.horizontalGradient(
                            colors = listOf(color.copy(alpha = 0.7f), color)
                        )
                    )
            )
        }
    }
}

/* ── 分区卡片 ── */
@Composable
fun SectionCard(
    modifier: Modifier = Modifier,
    title: String? = null,
    content: @Composable ColumnScope.() -> Unit
) {
    Card(
        modifier = modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.medium,
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 1.dp)
    ) {
        Column(Modifier.padding(12.dp)) {
            if (title != null) {
                Text(
                    title.uppercase(),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.primary,
                    fontWeight = FontWeight.Bold,
                    letterSpacing = 1.sp
                )
                Spacer(Modifier.height(12.dp))
            }
            content()
        }
    }
}

/* ── 信息行 ── */
@Composable
fun InfoRow(label: String, value: String, modifier: Modifier = Modifier) {
    Row(
        modifier.fillMaxWidth().padding(vertical = 4.dp),
        Arrangement.SpaceBetween
    ) {
        Text(
            label,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
        Text(
            value,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurface,
            fontWeight = FontWeight.Medium
        )
    }
}

/* ── 主操作按钮 ── */
@Composable
fun PrimaryButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    loading: Boolean = false
) {
    Button(
        onClick = onClick,
        enabled = enabled && !loading,
        modifier = modifier.fillMaxWidth().height(48.dp),
        shape = MaterialTheme.shapes.small,
        colors = androidx.compose.material3.ButtonDefaults.buttonColors(
            containerColor = MaterialTheme.colorScheme.primary,
            contentColor = MaterialTheme.colorScheme.onPrimary
        )
    ) {
        if (loading) {
            CircularProgressIndicator(
                modifier = Modifier.size(18.dp),
                color = MaterialTheme.colorScheme.onPrimary,
                strokeWidth = 2.dp
            )
        } else {
            Text(text, fontWeight = FontWeight.Bold)
        }
    }
}

/* ── 加载 / 空状态 ── */
@Composable
fun LoadingBox(modifier: Modifier = Modifier) {
    Box(modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        CircularProgressIndicator(
            color = MaterialTheme.colorScheme.primary,
            strokeWidth = 2.dp
        )
    }
}

@Composable
fun StateBox(
    message: String,
    modifier: Modifier = Modifier,
    onRetry: (() -> Unit)? = null
) {
    Box(modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Column(
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            Text(
                message,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                textAlign = androidx.compose.ui.text.style.TextAlign.Center
            )
            if (onRetry != null) {
                AppTextButton(onClick = onRetry) { 
                    Text("重试", color = MaterialTheme.colorScheme.primary, fontWeight = FontWeight.Bold)
                }
            }
        }
    }
}

@Composable
fun AppTextButton(onClick: () -> Unit, content: @Composable () -> Unit) {
    androidx.compose.material3.TextButton(onClick = onClick) { content() }
}

/**
 * 带动态小圆点后缀的文本，0→1→2→3 个点循环动画。
 * 用于等待/加载状态的友好提示。
 */
@Composable
fun AnimatedDotsText(
    label: String,
    color: Color = MaterialTheme.colorScheme.onSurfaceVariant,
    fontSize: androidx.compose.ui.unit.TextUnit = MaterialTheme.typography.bodySmall.fontSize,
    intervalMs: Long = 400L
) {
    var dotCount by remember { mutableIntStateOf(0) }
    LaunchedEffect(Unit) {
        while (true) {
            kotlinx.coroutines.delay(intervalMs)
            dotCount = (dotCount + 1) % 4
        }
    }
    Text(
        text = "$label${".".repeat(dotCount)}",
        color = color,
        fontSize = fontSize
    )
}

/**
 * 增强 Markdown 渲染：标题(多级)、加粗、斜体、行内代码、代码块、
 * 有序/无序列表、水平分割线、链接。
 * 解析结果通过 remember(text) 缓存，避免流式输出时重复解析。
 */
@Composable
fun MarkdownText(
    text: String,
    color: Color = MaterialTheme.colorScheme.onSurface,
    fontSize: androidx.compose.ui.unit.TextUnit = MaterialTheme.typography.bodyMedium.fontSize,
    lineHeight: androidx.compose.ui.unit.TextUnit = androidx.compose.ui.unit.TextUnit.Unspecified,
    modifier: Modifier = Modifier
) {
    val codeBg = MaterialTheme.colorScheme.surfaceVariant
    val monoFamily = androidx.compose.ui.text.font.FontFamily.Monospace
    val blocks = remember(text) { splitMarkdownBlocks(text) }
    val baseSizeValue = fontSize.value

    Column(modifier = modifier, verticalArrangement = Arrangement.spacedBy(2.dp)) {
        blocks.forEach { block ->
            when (block.first) {
                "code" -> {
                    Box(
                        Modifier.fillMaxWidth()
                            .background(codeBg, RoundedCornerShape(6.dp))
                            .padding(horizontal = 10.dp, vertical = 8.dp)
                    ) {
                        Text(
                            text = block.second,
                            fontFamily = monoFamily,
                            fontSize = (baseSizeValue - 1).coerceAtLeast(10f).sp,
                            color = color,
                            lineHeight = if (lineHeight != androidx.compose.ui.unit.TextUnit.Unspecified) lineHeight else fontSize
                        )
                    }
                }
                "h1" -> Text(
                    text = buildInlineMarkdown(block.second),
                    fontWeight = FontWeight.Black,
                    fontSize = (baseSizeValue + 5).sp,
                    color = color, lineHeight = lineHeight
                )
                "h2" -> Text(
                    text = buildInlineMarkdown(block.second),
                    fontWeight = FontWeight.Bold,
                    fontSize = (baseSizeValue + 3).sp,
                    color = color, lineHeight = lineHeight
                )
                "h3" -> Text(
                    text = buildInlineMarkdown(block.second),
                    fontWeight = FontWeight.Bold,
                    fontSize = (baseSizeValue + 1.5f).sp,
                    color = color, lineHeight = lineHeight
                )
                "hr" -> {
                    Spacer(Modifier.height(4.dp))
                    Box(
                        Modifier.fillMaxWidth().height(1.dp)
                            .background(MaterialTheme.colorScheme.outlineVariant)
                    )
                    Spacer(Modifier.height(4.dp))
                }
                "ul" -> Row(Modifier.fillMaxWidth().padding(start = 12.dp)) {
                    Text("\u2022 ", color = color, fontSize = fontSize)
                    Text(
                        text = buildInlineMarkdown(block.second),
                        fontSize = fontSize, color = color, lineHeight = lineHeight,
                        modifier = Modifier.weight(1f)
                    )
                }
                "ol" -> {
                    val dotIdx = block.second.indexOf(".\t")
                    val num = if (dotIdx > 0) block.second.substring(0, dotIdx) else "\u2022"
                    val content = if (dotIdx > 0) block.second.substring(dotIdx + 2) else block.second
                    Row(Modifier.fillMaxWidth().padding(start = 12.dp)) {
                        Text("$num ", color = color, fontSize = fontSize, fontWeight = FontWeight.SemiBold)
                        Text(
                            text = buildInlineMarkdown(content),
                            fontSize = fontSize, color = color, lineHeight = lineHeight,
                            modifier = Modifier.weight(1f)
                        )
                    }
                }
                else -> Text(
                    text = buildInlineMarkdown(block.second),
                    fontSize = fontSize, color = color, lineHeight = lineHeight
                )
            }
        }
    }
}

/**
 * 将 Markdown 文本拆分为 (type, content) 块序列。
 */
private fun splitMarkdownBlocks(text: String): List<Pair<String, String>> {
    val blocks = mutableListOf<Pair<String, String>>()
    val lines = text.lines()
    var i = 0
    while (i < lines.size) {
        val line = lines[i]
        val trimmed = line.trimStart()
        when {
            // 代码块
            trimmed.startsWith("```") -> {
                val codeLines = mutableListOf<String>()
                i++
                while (i < lines.size && !lines[i].trimStart().startsWith("```")) {
                    codeLines.add(lines[i]); i++
                }
                blocks.add("code" to codeLines.joinToString("\n"))
                if (i < lines.size) i++ // skip closing ```
            }
            // 水平分割线 ---  ***  ___
            trimmed.matches(Regex("^[-*_]{3,}\\s*$")) -> {
                blocks.add("hr" to ""); i++
            }
            // 标题（区分级别）
            trimmed.startsWith("### ") -> {
                blocks.add("h3" to trimmed.removePrefix("### ").trim()); i++
            }
            trimmed.startsWith("## ") -> {
                blocks.add("h2" to trimmed.removePrefix("## ").trim()); i++
            }
            trimmed.startsWith("# ") -> {
                blocks.add("h1" to trimmed.removePrefix("# ").trim()); i++
            }
            // 无序列表
            trimmed.startsWith("- ") || trimmed.startsWith("* ") -> {
                blocks.add("ul" to trimmed.drop(2)); i++
            }
            // 有序列表
            trimmed.matches(Regex("^\\d+\\.\\s.*")) -> {
                val dotPos = trimmed.indexOf(". ")
                val num = trimmed.substring(0, dotPos)
                val content = trimmed.substring(dotPos + 2)
                blocks.add("ol" to "$num\t$content"); i++
            }
            // 普通文本（合并连续非特殊行）
            else -> {
                val normalLines = mutableListOf<String>()
                while (i < lines.size) {
                    val l = lines[i]
                    val t = l.trimStart()
                    if (t.startsWith("```") || t.matches(Regex("^[-*_]{3,}\\s*$")) ||
                        t.startsWith("# ") || t.startsWith("## ") || t.startsWith("### ") ||
                        t.startsWith("- ") || t.startsWith("* ") ||
                        t.matches(Regex("^\\d+\\.\\s.*"))) break
                    normalLines.add(l); i++
                }
                blocks.add("text" to normalLines.joinToString("\n"))
            }
        }
    }
    return blocks
}

/**
 * 行内 Markdown 渲染：加粗、斜体、行内代码、链接。
 */
private fun buildInlineMarkdown(text: String): androidx.compose.ui.text.AnnotatedString {
    return androidx.compose.ui.text.buildAnnotatedString {
        var i = 0
        while (i < text.length) {
            when {
                // Bold: **text**
                i + 1 < text.length && text[i] == '*' && text[i + 1] == '*' -> {
                    val end = text.indexOf("**", i + 2)
                    if (end > 0) {
                        pushStyle(androidx.compose.ui.text.SpanStyle(fontWeight = FontWeight.Bold))
                        append(text.substring(i + 2, end))
                        pop(); i = end + 2
                    } else { append(text[i]); i++ }
                }
                // Italic: *text*  (not preceded by *)
                text[i] == '*' && (i + 1 < text.length && text[i + 1] != '*') -> {
                    val end = text.indexOf('*', i + 1)
                    if (end > 0 && (end + 1 >= text.length || text[end + 1] != '*')) {
                        pushStyle(androidx.compose.ui.text.SpanStyle(fontStyle = androidx.compose.ui.text.font.FontStyle.Italic))
                        append(text.substring(i + 1, end))
                        pop(); i = end + 1
                    } else { append(text[i]); i++ }
                }
                // Inline code: `code`
                text[i] == '`' -> {
                    val end = text.indexOf('`', i + 1)
                    if (end > 0) {
                        pushStyle(androidx.compose.ui.text.SpanStyle(
                            fontFamily = androidx.compose.ui.text.font.FontFamily.Monospace,
                            background = androidx.compose.ui.graphics.Color(0x1A000000)
                        ))
                        append(text.substring(i + 1, end))
                        pop(); i = end + 1
                    } else { append(text[i]); i++ }
                }
                // Link: [text](url)
                text[i] == '[' -> {
                    val closeB = text.indexOf(']', i + 1)
                    if (closeB > 0 && closeB + 1 < text.length && text[closeB + 1] == '(') {
                        val closeP = text.indexOf(')', closeB + 2)
                        if (closeP > 0) {
                            val linkText = text.substring(i + 1, closeB)
                            val url = text.substring(closeB + 2, closeP)
                            pushStyle(androidx.compose.ui.text.SpanStyle(
                                color = androidx.compose.ui.graphics.Color(0xFF4F7FFF),
                                textDecoration = androidx.compose.ui.text.style.TextDecoration.Underline
                            ))
                            pushStringAnnotation(tag = "URL", annotation = url)
                            append(linkText)
                            pop() // annotation
                            pop() // style
                            i = closeP + 1
                        } else { append(text[i]); i++ }
                    } else { append(text[i]); i++ }
                }
                else -> { append(text[i]); i++ }
            }
        }
    }
}
