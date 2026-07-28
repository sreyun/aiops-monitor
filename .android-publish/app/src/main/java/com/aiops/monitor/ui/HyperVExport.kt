package com.aiops.monitor.ui

import android.app.Activity
import android.content.Context
import android.content.ContextWrapper
import android.content.Intent
import android.print.PrintAttributes
import android.print.PrintManager
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.Toast
import androidx.core.content.FileProvider
import com.aiops.monitor.data.models.HyperVInventory
import java.io.File
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/**
 * Hyper-V 资产导出：Excel 兼容 CSV（UTF-8 BOM）+ 系统打印另存 PDF。
 * 字段尽量与 Web 端导出对齐（宿主机 / VM / 磁盘 / 网卡 / 检查点）。
 */
object HyperVExport {

    private fun stateText(s: String): String = when (s) {
        "Running" -> "运行中"; "Off" -> "已关机"; "Paused" -> "已暂停"; "Saved" -> "已保存"
        "Starting" -> "启动中"; "Stopping" -> "停止中"; "" -> "未知"; else -> s
    }

    private fun uptime(sec: Long): String {
        if (sec <= 0) return ""
        val d = sec / 86400; val h = (sec % 86400) / 3600; val m = (sec % 3600) / 60
        return when {
            d > 0 -> "${d}天${h}时"
            h > 0 -> "${h}时${m}分"
            else -> "${m}分"
        }
    }

    private fun gb(v: Double): String =
        if (v <= 0) "" else if (v < 1) "%.0f MB".format(v * 1024) else "%.1f GB".format(v)

    private fun stamp(): String =
        SimpleDateFormat("yyyy-MM-dd_HH-mm-ss", Locale.getDefault()).format(Date())

    private fun csvCell(v: Any?): String {
        val s = v?.toString() ?: ""
        return if (s.contains(',') || s.contains('"') || s.contains('\n') || s.contains('\r')) {
            "\"" + s.replace("\"", "\"\"") + "\""
        } else s
    }

    private fun csvLine(cols: List<Any?>): String = cols.joinToString(",") { csvCell(it) }

    fun buildCsv(inventories: List<HyperVInventory>): String {
        val sb = StringBuilder()
        // Excel 识别 UTF-8
        sb.append('\uFEFF')

        sb.appendLine("# Hyper-V 虚拟机资产清单")
        sb.appendLine("# 导出时间,${SimpleDateFormat("yyyy-MM-dd HH:mm:ss", Locale.getDefault()).format(Date())}")
        sb.appendLine("# 宿主机数,${inventories.size}")
        val allGuests = inventories.flatMap { inv -> inv.guests.orEmpty().map { inv to it } }
        sb.appendLine("# 虚拟机数,${allGuests.size}")
        sb.appendLine()

        sb.appendLine("## 宿主机汇总")
        sb.appendLine(csvLine(listOf("物理机名称", "Host ID", "虚拟机数", "运行中", "清单更新时间")))
        inventories.forEach { inv ->
            val guests = inv.guests.orEmpty()
            sb.appendLine(csvLine(listOf(
                inv.host_name.ifBlank { inv.host_id }, inv.host_id,
                guests.size, guests.count { it.state == "Running" }, inv.updated_at
            )))
        }
        sb.appendLine()

        sb.appendLine("## 虚拟机清单")
        sb.appendLine(csvLine(listOf(
            "物理机名称", "虚拟机名称", "运行状态", "健康", "运行时长", "vCPU",
            "占宿主CPU%", "内存已分配(MB)", "内存需求(MB)", "启动内存(MB)", "内存类型",
            "动态范围(MB)", "世代", "配置版本", "集成服务", "复制状态", "IP地址",
            "关联纳管主机", "硬盘数", "网卡数", "检查点数", "虚拟机ID"
        )))
        allGuests.forEach { (inv, g) ->
            val running = g.state == "Running"
            val memType = if (g.dynamic_mem_enabled) "动态" else "静态"
            val memRange = if (g.dynamic_mem_enabled) "${g.mem_min_mb.toInt()} ~ ${g.mem_max_mb.toInt()}" else ""
            val disks = g.disks.orEmpty()
            val nics = g.nics.orEmpty()
            val cps = g.checkpoints.orEmpty()
            val ips = g.ip_addresses.orEmpty().joinToString(", ")
                .ifBlank { nics.flatMap { it.ip_addresses.orEmpty() }.joinToString(", ") }
            sb.appendLine(csvLine(listOf(
                inv.host_name.ifBlank { inv.host_id },
                g.name,
                stateText(g.state),
                g.health.ifBlank { "OK" },
                if (running) uptime(g.uptime_sec) else "",
                if (g.processor_count > 0) g.processor_count else "",
                if (running) "%.1f".format(g.cpu_usage) else "",
                if (running && g.mem_assigned_mb > 0) g.mem_assigned_mb.toInt() else "",
                if (running) g.mem_demand_mb.toInt() else "",
                if (g.mem_startup_mb > 0) g.mem_startup_mb.toInt() else "",
                memType,
                memRange,
                if (g.generation > 0) "Gen${g.generation}" else "",
                g.version,
                g.integration_state,
                if (g.repl_state.isNotBlank() && g.repl_state != "Disabled")
                    "${g.repl_state}${if (g.repl_health.isNotBlank()) " / ${g.repl_health}" else ""}" else "",
                ips,
                g.linked_host_name ?: g.linked_host_id ?: "",
                disks.size.coerceAtLeast(g.vhd_count),
                nics.size.coerceAtLeast(if (g.ip_addresses.orEmpty().isNotEmpty()) 1 else 0),
                cps.size.coerceAtLeast(g.checkpoint_count),
                g.id
            )))
        }
        sb.appendLine()

        sb.appendLine("## 虚拟硬盘明细")
        sb.appendLine(csvLine(listOf("物理机名称", "虚拟机名称", "磁盘路径", "控制器", "占用空间")))
        allGuests.forEach { (inv, g) ->
            g.disks.orEmpty().forEach { d ->
                val ctrl = "${d.controller_type} ${d.controller_number}:${d.controller_location}".trim()
                sb.appendLine(csvLine(listOf(
                    inv.host_name.ifBlank { inv.host_id }, g.name, d.path, ctrl, gb(d.file_size_gb)
                )))
            }
        }
        sb.appendLine()

        sb.appendLine("## 网卡明细")
        sb.appendLine(csvLine(listOf("物理机名称", "虚拟机名称", "网卡名称", "MAC", "虚拟交换机", "网卡状态", "IP地址")))
        allGuests.forEach { (inv, g) ->
            g.nics.orEmpty().forEach { n ->
                sb.appendLine(csvLine(listOf(
                    inv.host_name.ifBlank { inv.host_id }, g.name, n.name, n.mac, n.switch,
                    n.status.ifBlank { if (n.connected) "Connected" else "" },
                    n.ip_addresses.orEmpty().joinToString(", ")
                )))
            }
        }
        sb.appendLine()

        sb.appendLine("## 检查点明细")
        sb.appendLine(csvLine(listOf("物理机名称", "虚拟机名称", "检查点名称", "创建时间")))
        allGuests.forEach { (inv, g) ->
            g.checkpoints.orEmpty().forEach { c ->
                sb.appendLine(csvLine(listOf(
                    inv.host_name.ifBlank { inv.host_id }, g.name, c.name,
                    c.created.replace("T", " ").take(19)
                )))
            }
        }

        return sb.toString()
    }

    fun buildHtml(inventories: List<HyperVInventory>): String {
        fun esc(s: String) = s
            .replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
            .replace("\"", "&quot;")

        val allGuests = inventories.flatMap { inv -> inv.guests.orEmpty().map { inv to it } }
        val sb = StringBuilder()
        sb.append("""<!DOCTYPE html><html><head><meta charset="utf-8"><title>Hyper-V 虚拟机资产清单</title>
<style>
body{font-family:sans-serif;font-size:11px;color:#111;margin:16px}
h1{font-size:18px;margin:0 0 6px} h2{font-size:14px;margin:18px 0 8px;border-bottom:1px solid #ccc;padding-bottom:4px}
.meta{color:#555;margin-bottom:12px} table{border-collapse:collapse;width:100%;margin-bottom:8px}
th,td{border:1px solid #ccc;padding:4px 6px;text-align:left;vertical-align:top;word-break:break-all}
th{background:#f3f4f6;font-weight:600} tr{page-break-inside:avoid}
</style></head><body>""")
        sb.append("<h1>Hyper-V 虚拟机资产清单</h1>")
        sb.append("<div class=\"meta\">导出时间：${esc(SimpleDateFormat("yyyy-MM-dd HH:mm:ss", Locale.getDefault()).format(Date()))}")
        sb.append(" · 宿主机 ${inventories.size} · 虚拟机 ${allGuests.size}</div>")

        fun table(title: String, heads: List<String>, rows: List<List<String>>) {
            if (rows.isEmpty()) return
            sb.append("<h2>${esc(title)}</h2><table><thead><tr>")
            heads.forEach { sb.append("<th>${esc(it)}</th>") }
            sb.append("</tr></thead><tbody>")
            rows.forEach { r ->
                sb.append("<tr>")
                r.forEach { sb.append("<td>${esc(it)}</td>") }
                sb.append("</tr>")
            }
            sb.append("</tbody></table>")
        }

        table(
            "宿主机汇总",
            listOf("物理机名称", "Host ID", "虚拟机数", "运行中", "清单更新时间"),
            inventories.map { inv ->
                val guests = inv.guests.orEmpty()
                listOf(
                    inv.host_name.ifBlank { inv.host_id }, inv.host_id,
                    guests.size.toString(), guests.count { it.state == "Running" }.toString(), inv.updated_at
                )
            }
        )

        table(
            "虚拟机清单",
            listOf(
                "物理机名称", "虚拟机名称", "运行状态", "健康", "运行时长", "vCPU",
                "占宿主CPU%", "内存已分配(MB)", "内存需求(MB)", "内存类型", "IP地址", "关联纳管主机"
            ),
            allGuests.map { (inv, g) ->
                val running = g.state == "Running"
                listOf(
                    inv.host_name.ifBlank { inv.host_id },
                    g.name,
                    stateText(g.state),
                    g.health.ifBlank { "OK" },
                    if (running) uptime(g.uptime_sec) else "—",
                    if (g.processor_count > 0) g.processor_count.toString() else "—",
                    if (running) "%.1f%%".format(g.cpu_usage) else "—",
                    if (running && g.mem_assigned_mb > 0) g.mem_assigned_mb.toInt().toString() else "—",
                    if (running) g.mem_demand_mb.toInt().toString() else "—",
                    if (g.dynamic_mem_enabled) "动态" else "静态",
                    g.ip_addresses.orEmpty().joinToString(", ").ifBlank { "—" },
                    (g.linked_host_name ?: g.linked_host_id ?: "—")
                )
            }
        )

        table(
            "虚拟硬盘明细",
            listOf("物理机名称", "虚拟机名称", "磁盘路径", "控制器", "占用空间"),
            allGuests.flatMap { (inv, g) ->
                g.disks.orEmpty().map { d ->
                    listOf(
                        inv.host_name.ifBlank { inv.host_id }, g.name, d.path,
                        "${d.controller_type} ${d.controller_number}:${d.controller_location}".trim(),
                        gb(d.file_size_gb).ifBlank { "—" }
                    )
                }
            }
        )

        table(
            "网卡明细",
            listOf("物理机名称", "虚拟机名称", "网卡名称", "MAC", "虚拟交换机", "状态", "IP"),
            allGuests.flatMap { (inv, g) ->
                g.nics.orEmpty().map { n ->
                    listOf(
                        inv.host_name.ifBlank { inv.host_id }, g.name, n.name, n.mac, n.switch,
                        n.status.ifBlank { if (n.connected) "Connected" else "—" },
                        n.ip_addresses.orEmpty().joinToString(", ").ifBlank { "—" }
                    )
                }
            }
        )

        table(
            "检查点明细",
            listOf("物理机名称", "虚拟机名称", "检查点名称", "创建时间"),
            allGuests.flatMap { (inv, g) ->
                g.checkpoints.orEmpty().map { c ->
                    listOf(
                        inv.host_name.ifBlank { inv.host_id }, g.name, c.name,
                        c.created.replace("T", " ").take(19)
                    )
                }
            }
        )

        sb.append("</body></html>")
        return sb.toString()
    }

    fun shareExcel(context: Context, inventories: List<HyperVInventory>) {
        if (inventories.isEmpty() || inventories.all { it.guests.orEmpty().isEmpty() }) {
            Toast.makeText(context, "暂无虚拟机数据可导出", Toast.LENGTH_SHORT).show()
            return
        }
        try {
            val dir = File(context.cacheDir, "exports").also { it.mkdirs() }
            val file = File(dir, "HyperV虚拟机资产_${stamp()}.csv")
            file.writeText(buildCsv(inventories), Charsets.UTF_8)
            val uri = FileProvider.getUriForFile(context, "${context.packageName}.fileprovider", file)
            val intent = Intent(Intent.ACTION_SEND).apply {
                type = "text/csv"
                putExtra(Intent.EXTRA_STREAM, uri)
                putExtra(Intent.EXTRA_SUBJECT, "Hyper-V 虚拟机资产清单")
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            }
            context.startActivity(Intent.createChooser(intent, "导出 Excel"))
        } catch (e: Exception) {
            Toast.makeText(context, "导出失败：${e.message}", Toast.LENGTH_SHORT).show()
        }
    }

    fun printPdf(context: Context, inventories: List<HyperVInventory>) {
        if (inventories.isEmpty() || inventories.all { it.guests.orEmpty().isEmpty() }) {
            Toast.makeText(context, "暂无虚拟机数据可导出", Toast.LENGTH_SHORT).show()
            return
        }
        val activity = context.findActivity()
        if (activity == null) {
            Toast.makeText(context, "无法打开打印界面", Toast.LENGTH_SHORT).show()
            return
        }
        val html = buildHtml(inventories)
        val webView = WebView(activity)
        webView.webViewClient = object : WebViewClient() {
            override fun onPageFinished(view: WebView?, url: String?) {
                val pm = activity.getSystemService(PrintManager::class.java) ?: run {
                    Toast.makeText(context, "系统不支持打印", Toast.LENGTH_SHORT).show()
                    return
                }
                val job = "HyperV虚拟机资产_${stamp()}"
                val adapter = webView.createPrintDocumentAdapter(job)
                pm.print(job, adapter, PrintAttributes.Builder()
                    .setMediaSize(PrintAttributes.MediaSize.ISO_A4)
                    .setMinMargins(PrintAttributes.Margins.NO_MARGINS)
                    .build())
            }
        }
        webView.loadDataWithBaseURL(null, html, "text/html", "UTF-8", null)
    }

    private fun Context.findActivity(): Activity? {
        var ctx: Context? = this
        while (ctx is ContextWrapper) {
            if (ctx is Activity) return ctx
            ctx = ctx.baseContext
        }
        return null
    }
}
