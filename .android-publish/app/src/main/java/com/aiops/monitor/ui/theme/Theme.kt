package com.aiops.monitor.ui.theme

import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

/* ── 深色方案（默认 / 监控 SaaS 行业标准） ── */
private val DarkColorScheme = darkColorScheme(
    primary = Color(0xFF4F7FFF),
    onPrimary = Color(0xFFFFFFFF),
    primaryContainer = Color(0xFF1E3166),
    onPrimaryContainer = Color(0xFFD6E2FF),
    secondary = Color(0xFF00A86B),
    onSecondary = Color(0xFFFFFFFF),
    secondaryContainer = Color(0xFF0F3A2C),
    onSecondaryContainer = Color(0xFFB6F3D8),
    tertiary = Color(0xFFF59E0B),
    onTertiary = Color(0xFF2A1C00),
    background = Color(0xFF080C14),
    onBackground = Color(0xFFE6E8F0),
    surface = Color(0xFF111722),
    onSurface = Color(0xFFE6E8F0),
    surfaceVariant = Color(0xFF192130),
    onSurfaceVariant = Color(0xFF9AA3B8),
    outline = Color(0xFF39445A),
    outlineVariant = Color(0xFF222C3D),
    error = Color(0xFFEF4444),
    onError = Color(0xFFFFFFFF),
    errorContainer = Color(0xFF3A1614),
    onErrorContainer = Color(0xFFFFDAD6)
)

/* ── 浅色方案（高饱和、有层次、绝不灰白） ── */
private val LightColorScheme = lightColorScheme(
    primary = Color(0xFF2563EB),        // 互联网产品常用高识别蓝
    onPrimary = Color(0xFFFFFFFF),
    primaryContainer = Color(0xFFDBEAFE),
    onPrimaryContainer = Color(0xFF163B8F),
    secondary = Color(0xFF059669),       // 翠绿
    onSecondary = Color(0xFFFFFFFF),
    secondaryContainer = Color(0xFFC8F5E6),
    onSecondaryContainer = Color(0xFF063B2E),
    tertiary = Color(0xFFD97706),        // 琥珀橙
    onTertiary = Color(0xFFFFFFFF),
    background = Color(0xFFF5F7FB),      // 微冷灰底
    onBackground = Color(0xFF111827),
    surface = Color(0xFFFFFFFF),
    onSurface = Color(0xFF111827),
    surfaceVariant = Color(0xFFF0F4FA),  // 卡片区分底
    onSurfaceVariant = Color(0xFF64748B),
    outline = Color(0xFFCBD5E1),
    outlineVariant = Color(0xFFE2E8F0),
    error = Color(0xFFD9261E),
    onError = Color(0xFFFFFFFF),
    errorContainer = Color(0xFFFDDCD8),
    onErrorContainer = Color(0xFF4A1010)
)

/* ── 圆角体系 ── */
val AppShapes = Shapes(
    extraSmall = RoundedCornerShape(8.dp),
    small = RoundedCornerShape(12.dp),
    medium = RoundedCornerShape(16.dp),
    large = RoundedCornerShape(22.dp),
    extraLarge = RoundedCornerShape(30.dp)
)

/* ── 移动端可读性基线：正文不低于 12sp，标签不低于 11sp ── */
private val AppTypography = Typography(
    headlineSmall = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontWeight = FontWeight.Black,
        fontSize = 22.sp,
        lineHeight = 29.sp
    ),
    titleLarge = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontWeight = FontWeight.Bold,
        fontSize = 20.sp,
        lineHeight = 27.sp
    ),
    titleMedium = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontWeight = FontWeight.SemiBold,
        fontSize = 16.sp,
        lineHeight = 23.sp
    ),
    titleSmall = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontWeight = FontWeight.Bold,
        fontSize = 14.sp,
        lineHeight = 20.sp
    ),
    bodyLarge = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontSize = 16.sp,
        lineHeight = 24.sp
    ),
    bodyMedium = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontSize = 14.sp,
        lineHeight = 21.sp
    ),
    bodySmall = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontSize = 12.sp,
        lineHeight = 18.sp
    ),
    labelLarge = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontWeight = FontWeight.Medium,
        fontSize = 13.sp,
        lineHeight = 18.sp
    ),
    labelMedium = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontWeight = FontWeight.Medium,
        fontSize = 12.sp,
        lineHeight = 17.sp
    ),
    labelSmall = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontWeight = FontWeight.Medium,
        fontSize = 11.sp,
        lineHeight = 15.sp
    )
)

@Composable
fun AIOpsMonitorTheme(
    darkTheme: Boolean = true,   // 监控工具默认深色
    content: @Composable () -> Unit
) {
    MaterialTheme(
        colorScheme = if (darkTheme) DarkColorScheme else LightColorScheme,
        shapes = AppShapes,
        typography = AppTypography,
        content = content
    )
}
