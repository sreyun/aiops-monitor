package com.aiops.monitor.ui

fun formatBytes(bytes: Long): String {
    if (bytes <= 0) return "0 B"
    val units = arrayOf("B", "KB", "MB", "GB", "TB")
    val exp = (kotlin.math.ln(bytes.toDouble()) / kotlin.math.ln(1024.0)).toInt()
        .coerceIn(0, units.size - 1)
    val value = bytes / Math.pow(1024.0, exp.toDouble())
    return "%.1f %s".format(value, units[exp])
}

fun formatRate(bytesPerSec: Double): String = formatBytes(bytesPerSec.toLong()) + "/s"

/** 当前登录用户是否为管理员（与 Web `isAdmin()` / 服务端 RoleAdmin 对齐）。 */
fun com.aiops.monitor.data.models.MeResponse?.isAdmin(): Boolean =
    this?.role.equals("admin", ignoreCase = true)
