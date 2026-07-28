package com.aiops.monitor.ui.viewmodel

import androidx.lifecycle.ViewModel
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.viewModelScope
import com.aiops.monitor.data.ApiClient
import com.aiops.monitor.data.DevicePublicIP
import com.aiops.monitor.data.models.Alert
import com.aiops.monitor.data.models.AlertAction
import com.aiops.monitor.data.models.AlertRecord
import com.aiops.monitor.data.models.AlertGovernance
import com.aiops.monitor.data.models.CheckUpsertRequest
import com.aiops.monitor.data.models.DupGroup
import com.aiops.monitor.data.models.Message
import com.aiops.monitor.data.models.TerminalSessionInfo
import com.aiops.monitor.data.models.TermFrame
import com.aiops.monitor.data.models.Host
import com.aiops.monitor.data.models.LoginRequest
import com.aiops.monitor.data.models.MeResponse
import com.aiops.monitor.data.models.Sample
import com.aiops.monitor.data.models.Summary
import com.aiops.monitor.data.models.WeatherResponse
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch
import okhttp3.ResponseBody
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.RequestBody.Companion.toRequestBody
import retrofit2.HttpException
import org.json.JSONObject
import android.util.Log
import com.aiops.monitor.data.TerminalClient
import com.aiops.monitor.data.models.AiChatRequest
import com.aiops.monitor.data.models.AiFilePart
import com.aiops.monitor.data.models.AiHistoryMessage
import com.aiops.monitor.data.models.AiImagePart
import com.aiops.monitor.data.models.AiMessage
import com.aiops.monitor.data.models.ObservabilityDataSource
import com.aiops.monitor.data.models.ApiSystem
import com.aiops.monitor.data.models.CheckPoint
import com.aiops.monitor.data.models.CreateIncidentRequest
import com.aiops.monitor.data.models.Incident
import com.aiops.monitor.data.models.DiagnosisChatMessage
import com.aiops.monitor.data.models.DiagnosisFeedbackRequest
import com.aiops.monitor.data.models.IncidentDiagnosisChatRequest
import com.aiops.monitor.data.models.IncidentDiagnosisRequest
import com.aiops.monitor.data.models.IncidentDiagnosisResponse
import com.aiops.monitor.data.models.InspectionReport
import com.aiops.monitor.data.models.LogDiagnosisRequest
import com.aiops.monitor.data.models.LogSearchResponse
import com.aiops.monitor.data.models.Playbook
import com.aiops.monitor.data.models.PlaybookExecution
import com.aiops.monitor.data.models.Probe
import com.aiops.monitor.data.models.SreOverview
import com.aiops.monitor.data.models.SloStatus
import com.aiops.monitor.data.models.ActivityLogEntry
import com.aiops.monitor.data.models.StoredLog
import com.aiops.monitor.data.models.Ticket
import com.aiops.monitor.data.models.RemediationRun
import com.aiops.monitor.data.models.PortForwardRule
import com.aiops.monitor.data.models.PortForwardCreateRequest
import com.aiops.monitor.data.models.PortForwardEditRequest
import com.aiops.monitor.data.models.PortForwardToggleRequest
import com.aiops.monitor.data.models.HTTPProxyConfig
import com.aiops.monitor.data.models.ContentAuditEvent
import com.aiops.monitor.data.models.DataHost
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.Job
import kotlinx.coroutines.isActive
import kotlinx.coroutines.withContext
import org.json.JSONTokener
import java.io.IOException
import java.net.SocketTimeoutException

class AuthViewModel : ViewModel() {
    sealed class LoginState {
        object Idle : LoginState()
        object Loading : LoginState()
        object Success : LoginState()
        /**
         * @param clearCode when true the UI must wipe the OTP field — used after
         * invalid/replay/network failures so the user cannot resubmit a consumed code.
         */
        data class MfaRequired(
            val username: String,
            val password: String,
            val error: String? = null,
            val clearCode: Boolean = false,
            val errorCode: String? = null
        ) : LoginState()
        data class Error(val message: String) : LoginState()
    }

    private val _state = MutableStateFlow<LoginState>(LoginState.Idle)
    val state: StateFlow<LoginState> = _state

    fun login(username: String, password: String, code: String = "") {
        if (_state.value is LoginState.Loading) return
        val totp = normalizeTotpCode(code)
        // Reject incomplete MFA submits early so we never burn a partial/stale code server-side.
        if (code.isNotBlank() && totp.length != 6) {
            _state.value = LoginState.MfaRequired(
                username = username,
                password = password,
                error = "请输入 Authenticator 当前显示的 6 位动态口令",
                clearCode = true,
                errorCode = "totp_invalid"
            )
            return
        }
        _state.value = LoginState.Loading
        viewModelScope.launch {
            try {
                val u = username.trim()
                if (totp.isEmpty()) ApiClient.clearSession()
                val resp = ApiClient.api.login(LoginRequest(u, password, code = totp))
                when {
                    resp.mfa_required -> _state.value = LoginState.MfaRequired(
                        username = username,
                        password = password,
                        error = null,
                        clearCode = true // freshly opened MFA step — empty field
                    )
                    resp.require_mfa_setup -> _state.value = LoginState.Error("账号需要先在网页端完成 MFA 设置")
                    // Server may already have issued a session cookie; treat as success so the
                    // app is not stuck half-logged-in while showing a blocking error.
                    resp.must_change_password -> {
                        if (ApiClient.cookieHeader().isNotBlank()) {
                            ApiClient.markAuthorized()
                            _state.value = LoginState.Success
                        } else {
                            _state.value = LoginState.Error("账号需要先在网页端修改初始密码")
                        }
                    }
                    resp.ok && ApiClient.cookieHeader().isNotBlank() -> {
                        ApiClient.markAuthorized()
                        _state.value = LoginState.Success
                    }
                    resp.ok -> _state.value = LoginState.Error("服务器未返回登录会话 Cookie")
                    else -> {
                        val errCode = resp.code
                        if (totp.isNotEmpty()) {
                            _state.value = LoginState.MfaRequired(
                                username = username,
                                password = password,
                                error = mfaErrorMessage(resp.error, errCode),
                                clearCode = true,
                                errorCode = errCode
                            )
                        } else {
                            _state.value = LoginState.Error(resp.error ?: "登录失败")
                        }
                    }
                }
            } catch (e: HttpException) {
                val parsed = parseHttpApiError(e)
                if (totp.isNotEmpty()) {
                    _state.value = LoginState.MfaRequired(
                        username = username,
                        password = password,
                        error = mfaErrorMessage(parsed.message, parsed.code),
                        clearCode = true,
                        errorCode = parsed.code
                    )
                } else {
                    _state.value = LoginState.Error(parsed.message)
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: SocketTimeoutException) {
                // Server may have already consumed the TOTP before the client timed out.
                if (totp.isNotEmpty()) {
                    _state.value = LoginState.MfaRequired(
                        username = username,
                        password = password,
                        error = "请求超时。动态口令可能已在服务端使用，请等待 Authenticator 刷新后输入新口令",
                        clearCode = true,
                        errorCode = "totp_replay"
                    )
                } else {
                    _state.value = LoginState.Error("请求超时，请检查网络后重试")
                }
            } catch (e: Exception) {
                if (totp.isNotEmpty()) {
                    _state.value = LoginState.MfaRequired(
                        username = username,
                        password = password,
                        error = "网络异常。若刚才已提交动态口令，请等待 Authenticator 刷新后输入新口令再试",
                        clearCode = true,
                        errorCode = "network"
                    )
                } else {
                    _state.value = LoginState.Error(e.message ?: "网络错误")
                }
            }
        }
    }

    fun reset() { _state.value = LoginState.Idle }
}

/** Digits-only TOTP, max 6 — strips spaces/dashes from autofill paste. */
internal fun normalizeTotpCode(code: String): String =
    code.filter { it.isDigit() }.take(6)

private data class HttpApiError(val message: String, val code: String?)

private fun parseHttpApiError(e: HttpException): HttpApiError {
    val body = try { e.response()?.errorBody()?.string() } catch (_: Exception) { null }
    return try {
        if (body.isNullOrBlank()) HttpApiError("HTTP ${e.code()}", null)
        else {
            val obj = JSONObject(body)
            val msg = obj.optString("error", "").ifBlank { "HTTP ${e.code()}" }
            val code = obj.optString("code", "").ifBlank { null }
            HttpApiError(msg, code)
        }
    } catch (_: Exception) {
        HttpApiError("HTTP ${e.code()}", null)
    }
}

private fun mfaErrorMessage(serverMsg: String?, code: String?): String {
    return when (code) {
        "totp_replay" -> serverMsg?.takeIf { it.isNotBlank() }
            ?: "该动态口令已使用，请等待 Authenticator 刷新后再输入新口令"
        "totp_invalid" -> serverMsg?.takeIf { it.isNotBlank() }
            ?: "动态验证码错误，请输入 Authenticator 当前显示的新口令"
        else -> serverMsg?.takeIf { it.isNotBlank() } ?: "动态验证码错误"
    }
}

class HostsViewModel : ViewModel() {
    private val _hosts = MutableStateFlow<List<Host>>(emptyList())
    val hosts: StateFlow<List<Host>> = _hosts

    private val _summary = MutableStateFlow<Summary?>(null)
    val summary: StateFlow<Summary?> = _summary

    private val _me = MutableStateFlow<MeResponse?>(null)
    val me: StateFlow<MeResponse?> = _me

    private val _weather = MutableStateFlow<WeatherResponse?>(null)
    val weather: StateFlow<WeatherResponse?> = _weather

    // ── SRE 驾驶舱关注点（尽力而为，老服务端 404 静默降级） ──
    private val _sreOverview = MutableStateFlow<SreOverview?>(null)
    val sreOverview: StateFlow<SreOverview?> = _sreOverview

    private val _openIncidents = MutableStateFlow<List<Incident>>(emptyList())
    val openIncidents: StateFlow<List<Incident>> = _openIncidents

    private val _activeAlerts = MutableStateFlow<List<Alert>>(emptyList())
    val activeAlerts: StateFlow<List<Alert>> = _activeAlerts

    private val _loading = MutableStateFlow(false)
    val loading: StateFlow<Boolean> = _loading

    private val _error = MutableStateFlow<String?>(null)
    val error: StateFlow<String?> = _error

    private val _lastUpdated = MutableStateFlow(0L)
    val lastUpdated: StateFlow<Long> = _lastUpdated

    fun load() {
        if (!ApiClient.isInitialized()) {
            _error.value = "API 未初始化，请先登录"
            return
        }
        if (!ApiClient.isSessionAlive()) {
            _me.value = null
            _error.value = "会话已失效，请重新登录"
            return
        }
        _error.value = null
        loadJob?.cancel()
        loadJob = viewModelScope.launch {
            _loading.value = true
            try {
                val hostsDeferred = async { ApiClient.api.hosts() }
                val summaryDeferred = async { ApiClient.api.summary() }
                val meDeferred = async { apiResult { ApiClient.api.me() } }
                // 天气：优先用手机自身公网 IP 定位（避免落到服务器机房/园区出口城市），
                // 探测失败再回退服务端默认逻辑。
                val weatherDeferred = async {
                    apiResult {
                        val deviceIP = DevicePublicIP.resolve()
                        if (deviceIP != null) ApiClient.api.weather(deviceIP)
                        else ApiClient.api.weather()
                    }
                }
                // SRE 关注点尽力而为：并入总览单刷新环，任一失败不影响主机/概览主数据。
                val overviewDeferred = async { apiResult { ApiClient.api.sreOverview() } }
                val incidentsDeferred = async { apiResult { ApiClient.api.incidents() } }
                val alertsDeferred = async { apiResult { ApiClient.api.alerts() } }
                if (!ApiClient.isSessionAlive()) {
                    _me.value = null
                    return@launch
                }
                _hosts.value = hostsDeferred.await()
                _summary.value = summaryDeferred.await()
                meDeferred.await().onSuccess { _me.value = it }
                    .onFailure { _me.value = null }
                weatherDeferred.await().onSuccess { if (it.ok) _weather.value = it else _weather.value = null }
                overviewDeferred.await().onSuccess { _sreOverview.value = it }
                incidentsDeferred.await().onSuccess { list -> _openIncidents.value = list.filter { it.status != "resolved" && it.status != "closed" } }
                alertsDeferred.await().onSuccess { list -> _activeAlerts.value = list }
                _lastUpdated.value = System.currentTimeMillis()
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                if (!ApiClient.isSessionAlive()) _me.value = null
                _error.value = e.message ?: "加载失败"
            } finally {
                _loading.value = false
            }
        }
    }

    private var loadJob: Job? = null

    private suspend fun <T> apiResult(block: suspend () -> T): Result<T> = try {
        Result.success(block())
    } catch (error: CancellationException) {
        throw error
    } catch (error: Exception) {
        Result.failure(error)
    }
}

class AlertsViewModel : ViewModel() {
    private val _alerts = MutableStateFlow<List<Alert>>(emptyList())
    val alerts: StateFlow<List<Alert>> = _alerts

    private val _loading = MutableStateFlow(false)
    val loading: StateFlow<Boolean> = _loading

    private val _error = MutableStateFlow<String?>(null)
    val error: StateFlow<String?> = _error

    private val _notice = MutableStateFlow<String?>(null)
    val notice: StateFlow<String?> = _notice

    private val _busyKeys = MutableStateFlow<Set<String>>(emptySet())
    val busyKeys: StateFlow<Set<String>> = _busyKeys

    // 告警历史 + 治理规则（新接入的服务端能力）
    private val _history = MutableStateFlow<List<AlertRecord>>(emptyList())
    val history: StateFlow<List<AlertRecord>> = _history

    private val _governance = MutableStateFlow<AlertGovernance?>(null)
    val governance: StateFlow<AlertGovernance?> = _governance

    fun loadHistory(status: String = "") {
        viewModelScope.launch {
            try { _history.value = ApiClient.api.alertHistory(200, status) }
            catch (e: CancellationException) { throw e }
            catch (_: Exception) { /* 历史尽力而为 */ }
        }
    }

    fun loadGovernance() {
        viewModelScope.launch {
            try { _governance.value = ApiClient.api.alertGovernance() }
            catch (e: CancellationException) { throw e }
            catch (_: Exception) { /* 治理规则尽力而为 */ }
        }
    }

    fun saveGovernance(gov: AlertGovernance, onDone: (Boolean, String?) -> Unit = { _, _ -> }) {
        viewModelScope.launch {
            try {
                ApiClient.api.setAlertGovernance(gov)
                _governance.value = gov
                onDone(true, null)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                onDone(false, e.message ?: "保存失败")
            }
        }
    }

    fun toggleSilenceEnabled(ruleId: String, enabled: Boolean, onDone: (Boolean, String?) -> Unit = { _, _ -> }) {
        val cur = _governance.value ?: return onDone(false, "无治理配置")
        val next = cur.copy(
            silence_rules = cur.silence_rules.orEmpty().map {
                if (it.id == ruleId) it.copy(enabled = enabled) else it
            }
        )
        saveGovernance(next, onDone)
    }

    fun toggleInhibitEnabled(ruleId: String, enabled: Boolean, onDone: (Boolean, String?) -> Unit = { _, _ -> }) {
        val cur = _governance.value ?: return onDone(false, "无治理配置")
        val next = cur.copy(
            inhibit_rules = cur.inhibit_rules.orEmpty().map {
                if (it.id == ruleId) it.copy(enabled = enabled) else it
            }
        )
        saveGovernance(next, onDone)
    }

    fun toggleRouteEnabled(ruleId: String, enabled: Boolean, onDone: (Boolean, String?) -> Unit = { _, _ -> }) {
        val cur = _governance.value ?: return onDone(false, "无治理配置")
        val next = cur.copy(
            routes = cur.routes.orEmpty().map {
                if (it.id == ruleId) it.copy(enabled = enabled) else it
            }
        )
        saveGovernance(next, onDone)
    }

    fun load() {
        if (!ApiClient.isInitialized()) {
            _error.value = "API 未初始化，请先登录"
            return
        }
        if (_loading.value) return
        _loading.value = true
        _error.value = null
        viewModelScope.launch {
            try {
                val a = ApiClient.api.alerts()
                _alerts.value = a
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _error.value = e.message ?: "加载失败"
            } finally {
                _loading.value = false
            }
        }
    }

    fun ack(alert: Alert, onDone: () -> Unit = {}) {
        alertAction(alert, "acknowledged", "告警已确认", "确认告警失败", onDone) {
            val response = ApiClient.api.ackAlert(AlertAction(alert.host_id, alert.type, alert.scope ?: ""))
            check(response["status"] == "ok") { response["error"] as? String ?: "服务器未确认操作结果" }
        }
    }

    fun silence(alert: Alert, onDone: () -> Unit = {}) {
        alertAction(alert, "silenced", "告警已静默", "静默告警失败", onDone) {
            val response = ApiClient.api.silenceAlert(AlertAction(alert.host_id, alert.type, alert.scope ?: ""))
            check(response["status"] == "ok") { response["error"] as? String ?: "服务器未确认操作结果" }
        }
    }

    private fun alertAction(
        alert: Alert,
        newStatus: String,
        successMessage: String,
        failureMessage: String,
        onDone: () -> Unit,
        action: suspend () -> Unit
    ) {
        val key = alertBusyKey(alert)
        if (key in _busyKeys.value) return
        _busyKeys.value += key
        _error.value = null
        viewModelScope.launch {
            try {
                action()
                // Apply the server state immediately so the result is visible before the refresh returns.
                _alerts.value = _alerts.value.map {
                    if (alertBusyKey(it) == key) it.copy(status = newStatus) else it
                }
                _notice.value = "$successMessage：${alert.hostname} / ${alert.type.uppercase()}"
                onDone()
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _error.value = e.message ?: failureMessage
            } finally {
                _busyKeys.value -= key
            }
        }
    }

    fun clearNotice() { _notice.value = null }
    fun clearError() { _error.value = null }

    private fun alertBusyKey(alert: Alert): String =
        "${alert.host_id}|${alert.type}|${alert.scope.orEmpty()}"
}

enum class ChartTimeRange(val label: String, val minutes: Int) {
    ONE_HOUR("1小时", 60),
    THREE_HOURS("3小时", 180),
    SIX_HOURS("6小时", 360),
    TWELVE_HOURS("12小时", 720),
    ONE_DAY("1天", 1_440),
    THREE_DAYS("3天", 4_320),
    SEVEN_DAYS("7天", 10_080),
    FOURTEEN_DAYS("14天", 20_160);
}

class HostDetailViewModel : ViewModel() {
    private val _host = MutableStateFlow<Host?>(null)
    val host: StateFlow<Host?> = _host

    private val _metrics = MutableStateFlow<Sample?>(null)
    val metrics: StateFlow<Sample?> = _metrics

    private val _history = MutableStateFlow<List<Sample>>(emptyList())
    val history: StateFlow<List<Sample>> = _history

    private val _chartTimeRange = MutableStateFlow(ChartTimeRange.ONE_HOUR)
    val chartTimeRange: StateFlow<ChartTimeRange> = _chartTimeRange

    private val _historyLoading = MutableStateFlow(false)
    val historyLoading: StateFlow<Boolean> = _historyLoading

    private val _loading = MutableStateFlow(false)
    val loading: StateFlow<Boolean> = _loading

    private val _error = MutableStateFlow<String?>(null)
    val error: StateFlow<String?> = _error

    private var currentHostId: String? = null

    fun load(hostId: String) {
        if (!ApiClient.isInitialized()) {
            _error.value = "API 未初始化，请先登录"
            return
        }
        if (_loading.value) return
        currentHostId = hostId
        _loading.value = true
        _error.value = null
        viewModelScope.launch {
            try {
                val hostsDeferred = async { ApiClient.api.hosts() }
                val samplesDeferred = async { apiResult { ApiClient.api.hostMetrics(hostId) } }
                val range = _chartTimeRange.value
                val historyDeferred = async { apiResult { loadHistoryRange(hostId, range) } }

                val samplesResult = samplesDeferred.await()
                samplesResult.getOrNull()?.maxByOrNull { it.timestamp }?.let { _metrics.value = it }
                _host.value = hostsDeferred.await().find { it.id == hostId }

                // Always use hostHistory with current time range for chart data.
                // Fall back to /metrics samples only when /history is unavailable.
                val historyResult = historyDeferred.await()
                _history.value = when {
                    historyResult.isSuccess && historyResult.getOrNull().orEmpty().size >= 2 ->
                        historyResult.getOrNull().orEmpty()
                    samplesResult.isSuccess ->
                        samplesResult.getOrNull().orEmpty().sortedBy { it.timestamp }
                    else -> _history.value
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _error.value = e.message ?: "加载失败"
            } finally {
                _loading.value = false
            }
        }
    }

    fun setCategory(cat: String, onDone: () -> Unit = {}) {
        val id = currentHostId ?: _host.value?.id ?: return
        val clean = cat.trim()
        viewModelScope.launch {
            try {
                ApiClient.api.setHostCategory(id, mapOf("category" to clean))
                _host.value = _host.value?.copy(category = clean.ifBlank { null })
                onDone()
            } catch (e: CancellationException) { throw e }
            catch (e: Exception) { _error.value = e.message ?: "设置分组失败" }
        }
    }

    fun setChartTimeRange(range: ChartTimeRange) {
        if (_chartTimeRange.value == range) return
        _chartTimeRange.value = range
        val hostId = currentHostId ?: return
        _historyLoading.value = true
        viewModelScope.launch {
            try {
                val result = apiResult { loadHistoryRange(hostId, range) }
                result.getOrNull()?.let { data ->
                    if (data.size >= 2) _history.value = data
                }
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                // keep previous data on failure
            } finally {
                _historyLoading.value = false
            }
        }
    }

    private suspend fun loadHistoryRange(hostId: String, range: ChartTimeRange): List<Sample> {
        val to = System.currentTimeMillis() / 1000
        val from = to - range.minutes.toLong() * 60
        return ApiClient.api.hostHistory(hostId, from, to).sortedBy { it.timestamp }
    }

    private suspend fun <T> apiResult(block: suspend () -> T): Result<T> = try {
        Result.success(block())
    } catch (e: CancellationException) {
        throw e
    } catch (e: Exception) {
        Result.failure(e)
    }
}

class TerminalViewModel : ViewModel() {
    private var client: TerminalClient? = null
    private val _output = MutableStateFlow("")
    val output: StateFlow<String> = _output
    private val _status = MutableStateFlow("idle")
    val status: StateFlow<String> = _status

    fun connect(hostId: String) {
        if (client != null) return
        client = TerminalClient(ApiClient.baseUrl, hostId, ApiClient.cookieHeader())
        viewModelScope.launch {
            launch { client?.output?.collect { _output.value = it } }
            launch { client?.status?.collect { _status.value = it } }
            client?.connect(120, 40)
        }
    }

    fun sendInput(input: String) { client?.sendInput(input) }
    fun resize(cols: Int, rows: Int) { client?.sendResize(cols, rows) }

    override fun onCleared() {
        client?.close()
        client = null
        super.onCleared()
    }
}

class TerminalPasswordViewModel : ViewModel() {
    sealed class SetState {
        object Idle : SetState()
        object Loading : SetState()
        object Success : SetState()
        data class MfaRequired(val message: String?, val clearCode: Boolean = true) : SetState()
        data class Error(val message: String, val clearCode: Boolean = false) : SetState()
    }

    private val _state = MutableStateFlow<SetState>(SetState.Idle)
    val state: StateFlow<SetState> = _state

    fun set(password: String, code: String = "") {
        if (_state.value is SetState.Loading) return
        val inMfa = _state.value is SetState.MfaRequired
        val totp = if (inMfa) normalizeTotpCode(code) else code
        if (inMfa && totp.length != 6) {
            _state.value = SetState.MfaRequired(
                message = "请输入 Authenticator 当前显示的 6 位动态口令",
                clearCode = true
            )
            return
        }
        _state.value = SetState.Loading
        viewModelScope.launch {
            try {
                val req = mutableMapOf<String, String>("password" to password)
                if (totp.isNotBlank()) req["code"] = totp

                val resp = ApiClient.api.setTerminalPassword(req)
                if (resp["ok"] == true) {
                    _state.value = SetState.Success
                } else {
                    val mfa = resp["mfa_required"] == true
                    if (mfa) {
                        _state.value = SetState.MfaRequired(
                            message = (resp["error"] as? String)
                                ?: "账户已启用 MFA，请清空上方验证框后输入 6 位动态口令",
                            clearCode = true
                        )
                    } else {
                        val errCode = resp["code"] as? String
                        val errMsg = (resp["error"] as? String) ?: "设置失败"
                        _state.value = SetState.Error(
                            message = mfaErrorMessage(errMsg, errCode),
                            clearCode = inMfa || errCode == "totp_replay" || errCode == "totp_invalid"
                        )
                    }
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: HttpException) {
                val parsed = parseHttpApiError(e)
                if (inMfa || parsed.code == "totp_replay" || parsed.code == "totp_invalid") {
                    _state.value = SetState.MfaRequired(
                        message = mfaErrorMessage(parsed.message, parsed.code),
                        clearCode = true
                    )
                } else {
                    _state.value = SetState.Error(parsed.message)
                }
            } catch (e: Exception) {
                _state.value = if (inMfa) {
                    SetState.MfaRequired(
                        message = "网络异常。动态口令可能已使用，请等待刷新后输入新口令",
                        clearCode = true
                    )
                } else {
                    SetState.Error(e.message ?: "网络错误")
                }
            }
        }
    }

    fun reset() {
        _state.value = SetState.Idle
    }
}

internal fun shouldUseLegacyAiEndpoint(statusCode: Int): Boolean = statusCode == 404 || statusCode == 405

internal fun sanitizeAiText(text: String): String = text
    .replace(Regex("(?i)hermes(?:\\s+agent)?"), "智能运维服务")

internal fun buildCapabilityAwarePrompt(
    message: String,
    dataSources: List<ObservabilityDataSource>
): String {
    val normalized = message.lowercase()
    val knowledgeKeywords = listOf(
        "文档", "文件", "上传", "知识库", "手册", "规范", "制度", "历史资料", "mcp", "网页", "url",
        "案例", "经验", "复盘", "故障报告", "运维手册", "最佳实践"
    )
    val dataSourceKeywords = listOf(
        "loki", "prometheus", "promql", "logql", "数据源", "日志源", "指标源",
        "grafana", "外部日志", "外部指标"
    )
    val alertKeywords = listOf(
        "告警", "报警", "异常", "故障", "宕机", "不可用", "超时",
        "严重", "warning", "critical", "alert"
    )
    val hostKeywords = listOf(
        "主机", "服务器", "节点", "机器", "cpu", "内存", "磁盘", "负载",
        "网络", "进程", "uptime", "在线", "离线"
    )
    val needsKnowledge = knowledgeKeywords.any(normalized::contains)
    val needsDataSource = dataSourceKeywords.any(normalized::contains)
    val needsAlerts = alertKeywords.any(normalized::contains)
    val needsHostInfo = hostKeywords.any(normalized::contains)

    val requirements = buildList {
        if (needsKnowledge) {
            add("涉及已上传文档、历史知识或 MCP 资料，回答前必须调用 search_similar_cases 检索知识记忆；没有命中时请明确说明，不得用通用知识冒充检索结果。")
        } else {
            add("如果问题依赖组织私有知识、Web 端上传材料或历史沉淀，而不是通用知识，请先调用 search_similar_cases 检索再回答。")
        }
        if (needsDataSource) {
            val connected = dataSources
                .filter { it.enabled && it.name.isNotBlank() }
                .joinToString("、") { "${it.name}(${it.type})" }
                .ifBlank { "当前 App 尚未读取到数据源清单" }
            add("涉及 Loki 或 Prometheus 时必须先调用 list_datasources，再调用 query_datasource 获取真实结果。已读取的数据源：$connected。")
        }
        if (needsAlerts) {
            add("涉及告警或异常状态时，请先调用 list_alerts 获取当前活跃告警列表，结合告警类型、级别和持续时间进行分析。")
        }
        if (needsHostInfo && !needsAlerts) {
            add("涉及主机状态或资源指标时，请调用 query_metrics 获取真实数据（metric=all），严禁编造或臆测数据。")
        }
        add("最终只向用户呈现结论、关键证据和数据来源，不要复述本段检索要求。")
    }
    return "$message\n\n【检索要求】\n${requirements.joinToString("\n") { "- $it" }}"
}

class AiCopilotViewModel : ViewModel() {
    private val defaultSuggestions = listOf(
        "分析当前异常主机和告警",
        "检查最近的错误日志并给出处置建议",
        "检索已上传文档中的运维规范",
        "汇总 Loki 与 Prometheus 中的异常信号",
        "汇总整体可用性风险"
    )

    private val _messages = MutableStateFlow<List<AiMessage>>(listOf(
        AiMessage("assistant", "您好，我是 AIOps 智能助手。我可以为您提供故障诊断和运维编排服务。")
    ))
    val messages: StateFlow<List<AiMessage>> = _messages

    private val _loading = MutableStateFlow(false)
    val loading: StateFlow<Boolean> = _loading

    private val _activity = MutableStateFlow("AI 思考中…")
    val activity: StateFlow<String> = _activity

    private val _suggestions = MutableStateFlow(defaultSuggestions)
    val suggestions: StateFlow<List<String>> = _suggestions

    private val _dataSources = MutableStateFlow<List<ObservabilityDataSource>>(emptyList())
    val dataSources: StateFlow<List<ObservabilityDataSource>> = _dataSources

    // 告警诊断上下文：AlertsScreen 设置后导航到 AiCopilotScreen 自动消费
    private var _pendingAlertDiagnosis: Alert? = null
    val hasPendingAlertDiagnosis: Boolean get() = _pendingAlertDiagnosis != null

    private var sessionId = 0L
    private var chatJob: Job? = null

    companion object {
        private const val MAX_RETRIES = 1
        private const val RETRY_DELAY_MS = 600L
    }

    fun refreshAiContext() {
        if (!ApiClient.isInitialized()) return
        viewModelScope.launch {
            val suggestionsDeferred = async {
                try {
                    ApiClient.aiApi.aiSuggestions()
                } catch (error: CancellationException) {
                    throw error
                } catch (_: Exception) {
                    null
                }
            }
            val sourcesDeferred = async {
                try {
                    ApiClient.aiApi.observabilityDataSources()
                } catch (error: CancellationException) {
                    throw error
                } catch (_: Exception) {
                    emptyList()
                }
            }
            suggestionsDeferred.await()?.let { response ->
                val loaded = (
                    response.dynamic.take(2) +
                        defaultSuggestions.drop(2).take(2) +
                        response.curated
                    ).distinct().take(6)
                if (loaded.isNotEmpty()) _suggestions.value = loaded.map(::sanitizeAiText)
            }
            _dataSources.value = sourcesDeferred.await().filter { it.enabled }
        }
    }

    private suspend fun <T> withRetry(
        tag: String,
        block: suspend () -> T
    ): Result<T> {
        var lastException: Exception? = null
        for (attempt in 0..MAX_RETRIES) {
            try {
                return Result.success(block())
            } catch (e: CancellationException) {
                throw e
            } catch (e: HttpException) {
                val code = e.code()
                if (code !in setOf(429, 502, 503, 504)) {
                    return Result.failure(e)
                }
                lastException = e
                if (attempt < MAX_RETRIES) {
                    Log.w("AiCopilotVM", "$tag 失败 (HTTP $code)，第 ${attempt + 1} 次重试...")
                    delay(RETRY_DELAY_MS)
                }
            } catch (e: SocketTimeoutException) {
                // 流式请求超时后自动重放会重复消耗模型与工具资源，直接允许用户重试。
                return Result.failure(e)
            } catch (e: java.net.ConnectException) {
                lastException = e
                if (attempt < MAX_RETRIES) {
                    Log.w("AiCopilotVM", "$tag 连接失败，第 ${attempt + 1} 次重试...")
                    delay(RETRY_DELAY_MS)
                }
            } catch (e: java.net.UnknownHostException) {
                return Result.failure(e)
            } catch (e: Exception) {
                return Result.failure(e)
            }
        }
        return Result.failure(lastException ?: Exception("未知错误"))
    }

    private fun formatError(e: Throwable): String {
        val serverUrl = ApiClient.serverHost
        return when (e) {
            is java.net.UnknownHostException -> "❌ 无法解析服务器域名\n地址: $serverUrl\n请检查服务器地址配置"
            is HttpException -> "❌ 服务器返回 HTTP ${e.code()} 错误\n地址: $serverUrl\n请稍后重试"
            is SocketTimeoutException -> "⏱️ 智能分析超时\n地址: $serverUrl\n服务端工具查询可能耗时较长，请稍后重试"
            is java.net.ConnectException -> "❌ 无法建立连接\n地址: $serverUrl\n服务器可能未启动或网络不通"
            is IOException -> "❌ AI 服务异常: ${sanitizeAiText(e.message ?: "未知错误")}\n地址: $serverUrl"
            else -> "❌ 网络异常: ${sanitizeAiText(e.message ?: "未知错误")}\n地址: $serverUrl"
        }.let(::sanitizeAiText)
    }

    fun sendMessage(
        text: String,
        context: Map<String, String>? = null,
        images: List<AiImagePart> = emptyList(),
        files: List<AiFilePart> = emptyList()
    ) {
        val contextHint = context?.entries
            ?.joinToString("，") { "${it.key}=${it.value}" }
            ?.takeIf { it.isNotBlank() }
        val baseText = if (contextHint == null) text else "$text\n\n上下文：$contextHint"
        val requestText = buildCapabilityAwarePrompt(baseText, _dataSources.value)
        val display = buildString {
            append(text.ifBlank { if (images.isNotEmpty() || files.isNotEmpty()) "（附件）" else "" })
            if (images.isNotEmpty()) append("\n[图片 ×${images.size}]")
            if (files.isNotEmpty()) append("\n[文件：${files.joinToString { it.name }}]")
        }
        sendInternal(displayText = display, requestText = requestText.ifBlank { "请分析附件内容" }, responseType = "text", images = images, files = files)
    }

    fun diagnose(hostId: String) {
        sendInternal(
            displayText = "分析主机 $hostId 的状态",
            requestText = "请对主机 ID $hostId 做一次完整运维诊断：检查在线状态、关键指标、告警与近期日志，并给出按优先级排序的处置建议。",
            responseType = "diagnosis"
        )
    }

    /**
     * AlertsScreen 调用：设置待诊断的告警，AiCopilotScreen 进入后通过 processPendingAlert() 消费。
     */
    fun setPendingAlertDiagnosis(alert: Alert) {
        _pendingAlertDiagnosis = alert
    }

    /**
     * AiCopilotScreen 的 LaunchedEffect 中调用：若存在待诊断告警，自动触发带告警上下文的 AI 诊断。
     * 返回 true 表示已消费挂起告警（调用方应跳过 initialHostId 等默认诊断）。
     */
    fun processPendingAlert(): Boolean {
        val alert = _pendingAlertDiagnosis ?: return false
        _pendingAlertDiagnosis = null
        diagnoseAlert(alert)
        return true
    }

    /**
     * 针对特定告警发起 AI 诊断：携带完整告警上下文（级别、类型、主机名、消息、持续时间等）。
     */
    fun diagnoseAlert(alert: Alert) {
        val levelLabel = when (alert.level.lowercase()) {
            "critical" -> "严重"; "warning" -> "警告"; else -> alert.level
        }
        val durationText = if ((alert.since ?: 0) > 0) {
            val since = alert.since ?: 0
            val seconds = System.currentTimeMillis() / 1000 - since
            when {
                seconds >= 86400 -> "${seconds / 86400}天${(seconds % 86400) / 3600}小时"
                seconds >= 3600 -> "${seconds / 3600}小时${(seconds % 3600) / 60}分钟"
                seconds >= 60 -> "${seconds / 60}分钟"
                else -> "${seconds}秒"
            }
        } else "未知"
        val context = buildString {
            appendLine("【告警上下文】")
            appendLine("- 告警级别：$levelLabel")
            appendLine("- 告警类型：${alert.type}")
            appendLine("- 主机名：${alert.hostname}（host_id=${alert.host_id}）")
            if (!alert.ip.isNullOrBlank()) appendLine("- 主机 IP：${alert.ip}")
            appendLine("- 告警消息：${alert.message}")
            if (!alert.scope.isNullOrBlank()) appendLine("- 告警范围：${alert.scope}")
            appendLine("- 告警值：${alert.value}")
            appendLine("- 持续时间：$durationText")
            appendLine()
            appendLine("请基于以上告警上下文进行深度分析：")
            appendLine("1. 查询该主机当前所有性能指标（CPU/内存/磁盘/负载/网络/IO）")
            appendLine("2. 检索该主机最近 30 分钟的 error/warn 级别日志")
            appendLine("3. 查看该主机当前所有活跃告警")
            appendLine("4. 在历史案例库中搜索相似故障的处理经验")
            appendLine("5. 综合分析可能的根因、影响范围和风险等级")
            appendLine("6. 给出按优先级排序的处置建议")
        }
        sendInternal(
            displayText = "AI 诊断告警：${alert.hostname} 的 ${levelLabel} ${alert.type.uppercase()}",
            requestText = context,
            responseType = "diagnosis"
        )
    }

    private fun sendInternal(
        displayText: String,
        requestText: String,
        responseType: String,
        images: List<AiImagePart> = emptyList(),
        files: List<AiFilePart> = emptyList()
    ) {
        if ((requestText.isBlank() && images.isEmpty() && files.isEmpty()) || _loading.value) return
        val history = _messages.value
            .drop(1)
            .filter { it.role == "user" || (it.role == "assistant" && it.type != "error" && it.content.isNotBlank()) }
            .takeLast(10)
            .map { AiHistoryMessage(it.role, it.content) }
        _messages.value = _messages.value + AiMessage("user", displayText) +
            AiMessage("assistant", "", type = "streaming")
        _loading.value = true
        _activity.value = "正在连接智能运维服务"

        chatJob = viewModelScope.launch {
            val request = AiChatRequest(
                message = requestText,
                session_id = sessionId,
                history = history,
                stream = true,
                images = images,
                files = files
            )
            try {
                val result = withRetry("AI 对话") {
                    replaceStreaming("")
                    consumeAIStream(request)
                }
                result.fold(
                    onSuccess = { answer -> replaceStreaming(sanitizeAiText(answer), responseType) },
                    onFailure = { error -> replaceStreaming(formatError(error), "error") }
                )
            } catch (_: CancellationException) {
                val partial = streamingContent()
                replaceStreaming(partial.ifBlank { "已停止生成" }, "text")
            } finally {
                _activity.value = "AI 思考中"
                _loading.value = false
                chatJob = null
            }
        }
    }

    fun stopGeneration() {
        chatJob?.cancel()
    }

    private fun streamingContent(): String = _messages.value
        .lastOrNull { it.role == "assistant" && it.type == "streaming" }
        ?.content.orEmpty()

    private fun replaceStreaming(content: String, type: String = "streaming") {
        val list = _messages.value.toMutableList()
        val index = list.indexOfLast { it.role == "assistant" && it.type == "streaming" }
        if (index >= 0) list[index] = list[index].copy(content = content, type = type)
        _messages.value = list
    }

    private suspend fun consumeAIStream(request: AiChatRequest): String = withContext(Dispatchers.IO) {
        val body = try {
            ApiClient.aiApi.aiAgentChat(request)
        } catch (error: HttpException) {
            if (!shouldUseLegacyAiEndpoint(error.code())) throw error
            ApiClient.aiApi.legacyAiChat(request.copy(session_id = 0))
        }

        body.use { responseBody ->
            _activity.value = "已连接，正在分析问题"
            val source = responseBody.source()
            val answer = StringBuilder()
            val raw = StringBuilder()
            var streamError: String? = null
            var lastUiUpdate = 0L
            var lastUiLength = 0
            while (true) {
                val line = source.readUtf8Line() ?: break
                if (!line.startsWith("data:")) {
                    if (line.isNotBlank()) raw.appendLine(line)
                    continue
                }
                val payload = line.removePrefix("data:").trim()
                if (payload.isBlank() || payload == "[DONE]") continue
                try {
                    val json = JSONObject(payload)
                    when {
                        json.has("error") -> streamError = json.optString("error", "AI 服务返回错误")
                        json.has("session_id") -> sessionId = json.optLong("session_id", sessionId)
                        json.has("tool") -> {
                            val tool = json.optJSONObject("tool")
                            val name = tool?.optString("name").orEmpty()
                            val state = tool?.optString("state").orEmpty()
                            if (name.isNotBlank()) {
                                val label = toolDisplayName(name)
                                _activity.value = when (state) {
                                    "ok" -> "$label 已完成，正在整理结果…"
                                    "err" -> "$label 执行异常，正在继续分析…"
                                    else -> "正在调用$label…"
                                }
                            }
                        }
                        json.has("delta") -> {
                            answer.append(json.optString("delta"))
                            val now = System.nanoTime()
                            if (lastUiUpdate == 0L || now - lastUiUpdate >= 30_000_000L || answer.length - lastUiLength >= 48) {
                                replaceStreaming(sanitizeAiText(answer.toString()))
                                lastUiUpdate = now
                                lastUiLength = answer.length
                            }
                        }
                        json.has("answer") -> answer.append(json.optString("answer"))
                        json.has("content") -> answer.append(json.optString("content"))
                    }
                } catch (_: Exception) {
                    raw.appendLine(payload)
                }
            }
            streamError?.let { throw IOException(sanitizeAiText(it)) }
            if (answer.isNotBlank()) {
                replaceStreaming(sanitizeAiText(answer.toString()))
                return@withContext answer.toString()
            }

            val rawText = raw.toString().trim()
            if (rawText.isNotBlank()) {
                val parsed = try { JSONTokener(rawText).nextValue() } catch (_: Exception) { rawText }
                when (parsed) {
                    is JSONObject -> {
                        parsed.optString("error").takeIf { it.isNotBlank() }?.let { throw IOException(it) }
                        listOf("answer", "content", "message", "result")
                            .firstNotNullOfOrNull { key -> parsed.optString(key).takeIf { it.isNotBlank() } }
                            ?.let { return@withContext it }
                    }
                    is String -> return@withContext parsed
                }
            }
            throw IOException("服务器返回了空的 AI 响应")
        }
    }

    private fun toolDisplayName(name: String): String = when (name) {
        "query_metrics" -> "指标查询"
        "search_logs" -> "日志检索"
        "list_alerts" -> "告警查询"
        "search_similar_cases" -> "知识库检索"
        "run_diagnostic" -> "主机诊断"
        "run_python_action" -> "诊断分析"
        "list_datasources" -> "数据源发现"
        "query_datasource" -> "外部数据查询"
        else -> "分析工具"
    }

    fun clearHistory() {
        chatJob?.cancel()
        sessionId = 0
        _pendingAlertDiagnosis = null
        _loading.value = false
        _messages.value = listOf(AiMessage("assistant", "会话已清空。"))
    }

    /** 登出 / 会话失效：取消进行中请求并复位对话，避免跨账号残留。 */
    fun onSessionEnded() {
        chatJob?.cancel()
        sessionId = 0
        _pendingAlertDiagnosis = null
        _loading.value = false
        _activity.value = "AI 思考中…"
        _messages.value = listOf(
            AiMessage("assistant", "您好，我是 AIOps 智能助手。我可以为您提供故障诊断和运维编排服务。")
        )
    }
}

data class MonitorTrend(
    val title: String,
    val points: List<CheckPoint>
)

class MonitorViewModel : ViewModel() {
    private val _probes = MutableStateFlow<List<Probe>>(emptyList())
    val probes: StateFlow<List<Probe>> = _probes

    private val _apiSystems = MutableStateFlow<List<ApiSystem>>(emptyList())
    val apiSystems: StateFlow<List<ApiSystem>> = _apiSystems

    private val _loading = MutableStateFlow(false)
    val loading: StateFlow<Boolean> = _loading

    private val _error = MutableStateFlow<String?>(null)
    val error: StateFlow<String?> = _error

    private val _testingIds = MutableStateFlow<Set<String>>(emptySet())
    val testingIds: StateFlow<Set<String>> = _testingIds

    private val _trend = MutableStateFlow<MonitorTrend?>(null)
    val trend: StateFlow<MonitorTrend?> = _trend

    private val _trendLoading = MutableStateFlow(false)
    val trendLoading: StateFlow<Boolean> = _trendLoading

    private val _hostMap = MutableStateFlow<Map<String, String>>(emptyMap())
    val hostMap: StateFlow<Map<String, String>> = _hostMap

    private val _isAdmin = MutableStateFlow(false)
    val isAdmin: StateFlow<Boolean> = _isAdmin

    fun load() {
        if (!ApiClient.isInitialized()) {
            _error.value = "API 未初始化，请先登录"
            return
        }
        if (_loading.value) return
        _loading.value = true
        _error.value = null
        viewModelScope.launch {
            try {
                val probesDeferred = async { apiResult { ApiClient.api.probes() } }
                val monitorsDeferred = async { apiResult { ApiClient.api.apiMonitors() } }
                val hostsDeferred = async { apiResult { ApiClient.api.hosts() } }
                val meDeferred = async { apiResult { ApiClient.api.me() } }
                val errors = mutableListOf<String>()
                probesDeferred.await().fold(
                    onSuccess = { _probes.value = it },
                    onFailure = { errors += "拨测加载失败: ${it.message ?: "未知错误"}" }
                )
                monitorsDeferred.await().fold(
                    onSuccess = { _apiSystems.value = it.systems },
                    onFailure = { errors += "API 监控加载失败: ${it.message ?: "未知错误"}" }
                )
                hostsDeferred.await().fold(
                    onSuccess = { hosts -> _hostMap.value = hosts.associate { it.id to it.hostname } },
                    onFailure = { /* 主机列表加载失败不影响拨测展示 */ }
                )
                meDeferred.await().fold(
                    onSuccess = { _isAdmin.value = it.role.equals("admin", ignoreCase = true) },
                    onFailure = { /* 角色探测失败时按非管理员处理，写接口仍由服务端 403 兜底 */ }
                )
                _error.value = errors.takeIf { it.isNotEmpty() }?.joinToString("；")
            } catch (e: CancellationException) {
                // 协程被取消，正常行为，不设置错误状态
                throw e
            } catch (e: Exception) {
                _error.value = e.message ?: "加载失败"
            } finally {
                _loading.value = false
            }
        }
    }

    private suspend fun <T> apiResult(block: suspend () -> T): Result<T> = try {
        Result.success(block())
    } catch (error: CancellationException) {
        throw error
    } catch (error: Exception) {
        Result.failure(error)
    }

    fun upsertProbe(req: CheckUpsertRequest, onDone: () -> Unit = {}) {
        viewModelScope.launch {
            try { ApiClient.api.upsertCheck(req); load(); onDone() }
            catch (e: CancellationException) { throw e }
            catch (e: Exception) { _error.value = e.message ?: "保存失败" }
        }
    }

    fun deleteProbe(id: String, onDone: () -> Unit = {}) {
        viewModelScope.launch {
            try {
                ApiClient.api.deleteCheck(id)
                _probes.value = _probes.value.filterNot { it.id == id }
                onDone()
            } catch (e: CancellationException) { throw e }
            catch (e: Exception) { _error.value = e.message ?: "删除失败" }
        }
    }

    fun deleteApiSystem(id: String, onDone: () -> Unit = {}) {
        viewModelScope.launch {
            try {
                ApiClient.api.deleteApiSystem(id)
                _apiSystems.value = _apiSystems.value.filterNot { it.id == id }
                onDone()
            } catch (e: CancellationException) { throw e }
            catch (e: Exception) { _error.value = e.message ?: "删除失败" }
        }
    }

    // ── DNS 解析延迟阈值（全局 thresholds，经 /config 读写） ──
    private val _dnsWarnMs = MutableStateFlow(500.0)
    val dnsWarnMs: StateFlow<Double> = _dnsWarnMs
    private val _dnsCritMs = MutableStateFlow(2000.0)
    val dnsCritMs: StateFlow<Double> = _dnsCritMs

    fun loadDnsThresholds() {
        viewModelScope.launch {
            try {
                val th = JSONObject(ApiClient.api.serverConfigRaw().string()).optJSONObject("thresholds")
                if (th != null) {
                    _dnsWarnMs.value = th.optDouble("check_dns_timeout_warn", 500.0)
                    _dnsCritMs.value = th.optDouble("check_dns_timeout_crit", 2000.0)
                }
            } catch (e: CancellationException) { throw e } catch (_: Exception) {}
        }
    }

    /** 读全量配置 → 仅覆盖两个 DNS 阈值 → 回存。org.json 保留整数类型，服务端 mergeSecrets 保留密钥。仅管理员可写（与 Web/服务端 RBAC 对齐）。 */
    fun saveDnsThresholds(warnMs: Double, critMs: Double, onDone: (Boolean) -> Unit = {}) {
        viewModelScope.launch {
            if (!_isAdmin.value) {
                _error.value = "仅管理员可修改告警阈值"
                onDone(false)
                return@launch
            }
            try {
                val cfg = JSONObject(ApiClient.api.serverConfigRaw().string())
                val th = cfg.optJSONObject("thresholds") ?: JSONObject().also { cfg.put("thresholds", it) }
                th.put("check_dns_timeout_warn", warnMs)
                th.put("check_dns_timeout_crit", critMs)
                val body = cfg.toString().toRequestBody("application/json; charset=utf-8".toMediaType())
                ApiClient.api.saveServerConfigRaw(body)
                _dnsWarnMs.value = warnMs; _dnsCritMs.value = critMs
                onDone(true)
            } catch (e: CancellationException) { throw e }
            catch (e: Exception) { _error.value = e.message ?: "保存失败"; onDone(false) }
        }
    }

    fun testProbe(id: String) {
        val key = "probe:$id"
        if (key in _testingIds.value) return
        _testingIds.value = _testingIds.value + key
        viewModelScope.launch {
            try {
                ApiClient.api.testProbe(id)
                // 后端立即返回、后台执行探测；等待结果落地后再回刷。
                delay(1800L)
                _probes.value = ApiClient.api.probes()
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _error.value = e.message ?: "拨测失败"
                Log.e("MonitorVM", "拨测失败: ${e.message}")
            } finally {
                _testingIds.value = _testingIds.value - key
            }
        }
    }

    fun testApiSystem(id: String) {
        val key = "system:$id"
        if (key in _testingIds.value) return
        _testingIds.value = _testingIds.value + key
        viewModelScope.launch {
            try {
                ApiClient.api.testApiSystem(id)
                delay(1800L)
                _apiSystems.value = ApiClient.api.apiMonitors().systems
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _error.value = e.message ?: "API 监控测试失败"
                Log.e("MonitorVM", "API 监控测试失败: ${e.message}")
            } finally {
                _testingIds.value = _testingIds.value - key
            }
        }
    }

    private val _trendTimeRange = MutableStateFlow(ChartTimeRange.ONE_HOUR)
    val trendTimeRange: StateFlow<ChartTimeRange> = _trendTimeRange

    private var lastTrendType: String? = null
    private var lastTrendId: String? = null
    private var lastTrendName: String? = null

    fun loadProbeHistory(id: String, name: String) {
        lastTrendType = "probe"
        lastTrendId = id
        lastTrendName = name
        _trendTimeRange.value = ChartTimeRange.ONE_HOUR
        loadTrend("$name · 拨测历史") { ApiClient.api.probeHistory(id, ChartTimeRange.ONE_HOUR.minutes) }
    }

    fun loadApiHistory(id: String, name: String) {
        lastTrendType = "api"
        lastTrendId = id
        lastTrendName = name
        _trendTimeRange.value = ChartTimeRange.ONE_HOUR
        loadTrend("$name · 接口性能历史") { ApiClient.api.apiMonitorHistory(id, ChartTimeRange.ONE_HOUR.minutes) }
    }

    fun setTrendTimeRange(range: ChartTimeRange) {
        if (_trendTimeRange.value == range) return
        _trendTimeRange.value = range
        val type = lastTrendType ?: return
        val id = lastTrendId ?: return
        val name = lastTrendName ?: return
        viewModelScope.launch {
            try {
                val points = when (type) {
                    "probe" -> ApiClient.api.probeHistory(id, range.minutes)
                    "api" -> ApiClient.api.apiMonitorHistory(id, range.minutes)
                    else -> return@launch
                }
                _trend.value = MonitorTrend(
                    if (type == "probe") "$name · 拨测历史" else "$name · 接口性能历史",
                    points.sortedBy { it.timestamp }
                )
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                _error.value = "历史趋势加载失败: ${error.message ?: "未知错误"}"
            }
        }
    }

    private fun loadTrend(title: String, block: suspend () -> List<CheckPoint>) {
        if (_trendLoading.value) return
        _trendLoading.value = true
        viewModelScope.launch {
            try {
                _trend.value = MonitorTrend(title, block().sortedBy { it.timestamp })
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                _error.value = "历史趋势加载失败: ${error.message ?: "未知错误"}"
            } finally {
                _trendLoading.value = false
            }
        }
    }

    fun closeTrend() {
        _trend.value = null
        lastTrendType = null
        lastTrendId = null
        lastTrendName = null
    }
}

data class IncidentDiagnosisUiState(
    val incident: Incident,
    val messages: List<DiagnosisChatMessage> = emptyList(),
    val running: Boolean = true,
    val activity: String = "正在读取事件上下文…",
    val includeTerminal: Boolean = false,
    val error: String? = null
)

class OperationsViewModel : ViewModel() {
    private var executionPollJob: Job? = null
    private var diagnosisPhaseJob: Job? = null
    private var diagnosisRequestJob: Job? = null
    private var currentIncidentId: Long = 0
    private val _overview = MutableStateFlow<SreOverview?>(null)
    val overview: StateFlow<SreOverview?> = _overview

    private val _incidents = MutableStateFlow<List<Incident>>(emptyList())
    val incidents: StateFlow<List<Incident>> = _incidents

    private val _logs = MutableStateFlow(LogSearchResponse())
    val logs: StateFlow<LogSearchResponse> = _logs

    private val _playbooks = MutableStateFlow<List<Playbook>>(emptyList())
    val playbooks: StateFlow<List<Playbook>> = _playbooks

    private val _executions = MutableStateFlow<List<PlaybookExecution>>(emptyList())
    val executions: StateFlow<List<PlaybookExecution>> = _executions

    private val _executionToOpen = MutableStateFlow<Long?>(null)
    val executionToOpen: StateFlow<Long?> = _executionToOpen

    private val _slos = MutableStateFlow<List<SloStatus>>(emptyList())
    val slos: StateFlow<List<SloStatus>> = _slos

    private val _tickets = MutableStateFlow<List<Ticket>>(emptyList())
    val tickets: StateFlow<List<Ticket>> = _tickets

    private val _remediationRuns = MutableStateFlow<List<RemediationRun>>(emptyList())
    val remediationRuns: StateFlow<List<RemediationRun>> = _remediationRuns

    private val _loading = MutableStateFlow(false)
    val loading: StateFlow<Boolean> = _loading

    private val _portForwards = MutableStateFlow<List<PortForwardRule>>(emptyList())
    val portForwards: StateFlow<List<PortForwardRule>> = _portForwards

    private val _portForwardsLoading = MutableStateFlow(false)
    val portForwardsLoading: StateFlow<Boolean> = _portForwardsLoading

    private val _hosts = MutableStateFlow<List<Host>>(emptyList())
    val hosts: StateFlow<List<Host>> = _hosts

    private val _httpProxies = MutableStateFlow<List<HTTPProxyConfig>>(emptyList())
    val httpProxies: StateFlow<List<HTTPProxyConfig>> = _httpProxies

    private val _terminalAuditLogs = MutableStateFlow<List<ActivityLogEntry>>(emptyList())
    val terminalAuditLogs: StateFlow<List<ActivityLogEntry>> = _terminalAuditLogs

    private val _terminalSessionsLoading = MutableStateFlow(false)
    val terminalSessionsLoading: StateFlow<Boolean> = _terminalSessionsLoading

    private val _logsLoading = MutableStateFlow(false)
    val logsLoading: StateFlow<Boolean> = _logsLoading

    private val _busyIds = MutableStateFlow<Set<String>>(emptySet())
    val busyIds: StateFlow<Set<String>> = _busyIds

    private val _error = MutableStateFlow<String?>(null)
    val error: StateFlow<String?> = _error

    private val _notice = MutableStateFlow<String?>(null)
    val notice: StateFlow<String?> = _notice

    private val _diagnosis = MutableStateFlow<InspectionReport?>(null)
    val diagnosis: StateFlow<InspectionReport?> = _diagnosis

    private val _incidentDiagnosis = MutableStateFlow<IncidentDiagnosisUiState?>(null)
    val incidentDiagnosis: StateFlow<IncidentDiagnosisUiState?> = _incidentDiagnosis

    fun load() {
        if (!ApiClient.isInitialized() || _loading.value) return
        _loading.value = true
        _error.value = null
        viewModelScope.launch {
            val errors = mutableListOf<String>()
            val overviewDeferred = async { apiResult { ApiClient.api.sreOverview() } }
            val incidentsDeferred = async { apiResult { ApiClient.api.incidents() } }
            val playbooksDeferred = async { apiResult { ApiClient.api.playbooks() } }
            val executionsDeferred = async { apiResult { ApiClient.api.playbookExecutions() } }
            val slosDeferred = async { apiResult { ApiClient.api.slos() } }
            val ticketsDeferred = async { apiResult { ApiClient.api.tickets() } }
            val remediationDeferred = async { apiResult { ApiClient.api.remediationRuns() } }

            overviewDeferred.await().fold({ _overview.value = it }, { errors += "SRE 概览: ${it.message}" })
            incidentsDeferred.await().fold({ _incidents.value = it }, { errors += "事件: ${it.message}" })
            playbooksDeferred.await().fold({ _playbooks.value = it }, { errors += "剧本: ${it.message}" })
            executionsDeferred.await().fold({ loaded ->
                _executions.value = loaded
                if (loaded.any { it.status == "running" }) monitorExecutions()
            }, { errors += "执行记录: ${it.message}" })
            slosDeferred.await().fold({ _slos.value = it }, { errors += "SLO: ${it.message}" })
            ticketsDeferred.await().fold({ _tickets.value = it }, { errors += "工单: ${it.message}" })
            remediationDeferred.await().fold({ _remediationRuns.value = it }, { errors += "自动修复: ${it.message}" })
            _error.value = errors.takeIf { it.isNotEmpty() }?.joinToString("；")
            _loading.value = false
        }
    }

    fun searchLogs(query: String, level: String, sinceMinutes: Int, page: Int = 1) {
        if (_logsLoading.value) return
        _logsLoading.value = true
        viewModelScope.launch {
            try {
                _logs.value = ApiClient.api.logs(
                    level = level,
                    query = query.trim(),
                    sinceMinutes = sinceMinutes,
                    page = page,
                    pageSize = 100
                )
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                _error.value = "日志检索失败: ${error.message ?: "未知错误"}"
            } finally {
                _logsLoading.value = false
            }
        }
    }

    fun loadTerminalSessions() {
        if (_terminalSessionsLoading.value) return
        _terminalSessionsLoading.value = true
        viewModelScope.launch {
            try {
                // 从活动日志接口获取终端命令审计记录（kind = "terminal"）
                val allActivity = ApiClient.api.activity()
                _terminalAuditLogs.value = allActivity.filter { it.kind == "terminal" }
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                _error.value = "终端审计加载失败: ${error.message ?: "未知错误"}"
            } finally {
                _terminalSessionsLoading.value = false
            }
        }
    }

    fun createIncident(title: String, severity: String, onDone: (Boolean) -> Unit = {}) {
        if (title.isBlank()) return
        action("incident:create", "事件已创建", onDone) {
            ApiClient.api.createIncident(CreateIncidentRequest(title.trim(), severity))
            refreshIncidents()
        }
    }

    fun ackIncident(id: Long) {
        action("incident:$id", "事件已确认") {
            replaceIncident(ApiClient.api.ackIncident(id))
            refreshOverview()
        }
    }

    fun resolveIncident(id: Long) {
        action("incident:$id", "事件已解决") {
            replaceIncident(ApiClient.api.resolveIncident(id))
            refreshOverview()
        }
    }

    fun escalateIncident(id: Long) {
        action("incident:$id", "事件已升级为工单") {
            ApiClient.api.escalateIncident(id)
            refreshIncidents()
            refreshOverview()
        }
    }

    /** 诊断请求重试：对超时/连接失败/5xx 进行 1 次重试，避免首次冷启动超时。 */
    private suspend fun <T> withDiagRetry(tag: String, block: suspend () -> T): Result<T> {
        var lastException: Exception? = null
        for (attempt in 0..1) {
            try {
                return Result.success(block())
            } catch (e: CancellationException) {
                throw e
            } catch (e: HttpException) {
                val code = e.code()
                if (code !in setOf(429, 502, 503, 504)) return Result.failure(e)
                lastException = e
                if (attempt < 1) {
                    Log.w("OpsVM", "$tag HTTP $code，重试中...")
                    delay(800L)
                }
            } catch (e: SocketTimeoutException) {
                lastException = e
                if (attempt < 1) {
                    Log.w("OpsVM", "$tag 超时，重试中...")
                    delay(1_000L)
                }
            } catch (e: java.net.ConnectException) {
                lastException = e
                if (attempt < 1) {
                    Log.w("OpsVM", "$tag 连接失败，重试中...")
                    delay(800L)
                }
            } catch (e: Exception) {
                return Result.failure(e)
            }
        }
        return Result.failure(lastException ?: Exception("未知错误"))
    }

    /**
     * 解析事件诊断响应：后端 AI 启用时返回 SSE 流，未启用时返回 JSON 对象。
     * 自动检测格式并统一输出为 IncidentDiagnosisResponse。
     */
    private suspend fun parseDiagnosisResponse(body: ResponseBody): IncidentDiagnosisResponse = withContext(Dispatchers.IO) {
        body.use { responseBody ->
            val source = responseBody.source()
            val answer = StringBuilder()
            val raw = StringBuilder()
            var streamError: String? = null
            var lastUiUpdate = 0L
            var lastUiLength = 0

            while (true) {
                val line = source.readUtf8Line() ?: break
                if (!line.startsWith("data:")) {
                    if (line.isNotBlank()) raw.appendLine(line)
                    continue
                }
                val payload = line.removePrefix("data:").trim()
                if (payload.isBlank() || payload == "[DONE]") continue
                try {
                    val json = JSONObject(payload)
                    when {
                        json.has("error") -> streamError = json.optString("error", "诊断服务返回错误")
                        json.has("delta") -> {
                            answer.append(json.optString("delta"))
                            val now = System.nanoTime()
                            if (lastUiUpdate == 0L || now - lastUiUpdate >= 30_000_000L || answer.length - lastUiLength >= 48) {
                                replaceIncidentDiagnosisStreaming(currentIncidentId, sanitizeAiText(answer.toString()))
                                updateIncidentDiagnosis(currentIncidentId) {
                                    it.copy(activity = "AI 正在生成诊断报告…")
                                }
                                lastUiUpdate = now
                                lastUiLength = answer.length
                            }
                        }
                        json.has("reply") -> answer.append(json.optString("reply"))
                        json.has("answer") -> answer.append(json.optString("answer"))
                        json.has("content") -> answer.append(json.optString("content"))
                        json.has("diagnosis") -> {
                            return@withContext IncidentDiagnosisResponse(
                                diagnosis = json.optString("diagnosis"),
                                source = json.optString("source", "heuristic")
                            )
                        }
                    }
                } catch (_: Exception) {
                    raw.appendLine(payload)
                }
            }

            streamError?.let { throw IOException(it) }

            if (answer.isNotBlank()) {
                return@withContext IncidentDiagnosisResponse(
                    diagnosis = sanitizeAiText(answer.toString()),
                    source = "ai"
                )
            }

            val rawText = raw.toString().trim()
            if (rawText.isNotBlank()) {
                val parsed = try { JSONTokener(rawText).nextValue() } catch (_: Exception) { rawText }
                when (parsed) {
                    is JSONObject -> {
                        val diag = parsed.optString("diagnosis", "")
                        if (diag.isNotBlank()) {
                            return@withContext IncidentDiagnosisResponse(
                                diagnosis = diag,
                                source = parsed.optString("source", "heuristic")
                            )
                        }
                        listOf("answer", "content", "message", "result")
                            .firstNotNullOfOrNull { key -> parsed.optString(key).takeIf { it.isNotBlank() } }
                            ?.let { return@withContext IncidentDiagnosisResponse(diagnosis = it, source = "ai") }
                    }
                    is String -> return@withContext IncidentDiagnosisResponse(diagnosis = parsed, source = "ai")
                }
            }

            throw IOException("服务器返回了空的诊断响应")
        }
    }

    /** Opens the incident-specific diagnosis workspace and streams SSE into the chat. */
    fun openIncidentDiagnosis(incident: Incident) {
        if (_incidentDiagnosis.value?.running == true) return
        val placeholder = DiagnosisChatMessage("assistant", "", System.currentTimeMillis() / 1000)
        _incidentDiagnosis.value = IncidentDiagnosisUiState(
            incident = incident,
            messages = listOf(placeholder),
            running = true,
            activity = "AI 正在诊断…"
        )
        diagnosisPhaseJob?.cancel()
        diagnosisPhaseJob = null
        diagnosisRequestJob?.cancel()
        diagnosisRequestJob = viewModelScope.launch {
            try {
                val persisted = apiResult { ApiClient.aiApi.incidentDiagnosisHistory(incident.id) }
                    .getOrNull()?.history.orEmpty()
                if (persisted.isNotEmpty()) {
                    updateIncidentDiagnosis(incident.id) {
                        it.copy(messages = persisted + placeholder)
                    }
                }
                currentIncidentId = incident.id
                val result = withDiagRetry("事件诊断") {
                    parseDiagnosisResponse(
                        ApiClient.aiApi.diagnoseIncident(incident.id, IncidentDiagnosisRequest(stream = true))
                    )
                }.getOrElse { throw it }
                check(result.diagnosis.isNotBlank()) { "服务器未返回诊断结论" }
                val sourceLabel = when (result.source.lowercase()) {
                    "ai" -> "AI 综合诊断"
                    "heuristic" -> "规则降级诊断"
                    else -> result.source.ifBlank { "诊断引擎" }
                }
                replaceIncidentDiagnosisStreaming(incident.id, "【$sourceLabel】\n${result.diagnosis}")
                updateIncidentDiagnosis(incident.id) {
                    it.copy(running = false, activity = "诊断已完成", error = null)
                }
                refreshIncidents()
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                updateIncidentDiagnosis(incident.id) {
                    it.copy(
                        running = false,
                        activity = "诊断未完成",
                        error = "AI 诊断失败：${formatOperationsError(error)}"
                    )
                }
            } finally {
                diagnosisRequestJob = null
            }
        }
    }

    fun setIncidentDiagnosisTerminalContext(enabled: Boolean) {
        val current = _incidentDiagnosis.value ?: return
        if (!current.running) _incidentDiagnosis.value = current.copy(includeTerminal = enabled)
    }

    fun sendIncidentDiagnosisMessage(
        message: String,
        images: List<AiImagePart> = emptyList(),
        files: List<AiFilePart> = emptyList()
    ) {
        val text = message.trim()
        val current = _incidentDiagnosis.value ?: return
        if ((text.isBlank() && images.isEmpty() && files.isEmpty()) || current.running) return
        val incidentId = current.incident.id
        val history = current.messages
            .filter { it.role == "user" || it.role == "assistant" }
            .takeLast(20)
            .map { AiHistoryMessage(it.role, it.content) }
        val display = buildString {
            append(text.ifBlank { "（附件）" })
            if (images.isNotEmpty()) append("\n[图片 ×${images.size}]")
            if (files.isNotEmpty()) append("\n[文件：${files.joinToString { it.name }}]")
        }
        val user = DiagnosisChatMessage("user", display, System.currentTimeMillis() / 1000)
        val placeholder = DiagnosisChatMessage("assistant", "", System.currentTimeMillis() / 1000)
        _incidentDiagnosis.value = current.copy(
            messages = current.messages + user + placeholder,
            running = true,
            activity = if (current.includeTerminal) "正在结合终端操作记录继续分析…" else "正在结合事件上下文继续分析…",
            error = null
        )
        diagnosisRequestJob?.cancel()
        diagnosisRequestJob = viewModelScope.launch {
            try {
                val answer = consumeIncidentDiagnosisStream(
                    incidentId,
                    IncidentDiagnosisChatRequest(
                        message = text.ifBlank { "请分析附件" },
                        history = history,
                        include_terminal = current.includeTerminal,
                        stream = true,
                        images = images,
                        files = files
                    )
                )
                check(answer.isNotBlank()) { "服务器返回了空的诊断回复" }
                replaceIncidentDiagnosisStreaming(incidentId, answer)
                updateIncidentDiagnosis(incidentId) { it.copy(running = false, activity = "追问分析已完成") }
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                replaceIncidentDiagnosisStreaming(incidentId, "")
                updateIncidentDiagnosis(incidentId) {
                    it.copy(running = false, activity = "追问未完成", error = "追问失败：${formatOperationsError(error)}")
                }
            } finally {
                diagnosisRequestJob = null
            }
        }
    }

    fun sendIncidentDiagnosisFeedback(helpful: Boolean) {
        val state = _incidentDiagnosis.value ?: return
        val messageIndex = state.messages.indexOfLast { it.role == "assistant" && it.content.isNotBlank() }
        if (messageIndex < 0) return
        viewModelScope.launch {
            try {
                ApiClient.aiApi.incidentDiagnosisFeedback(
                    state.incident.id,
                    DiagnosisFeedbackRequest(message_index = messageIndex, helpful = helpful)
                )
                _notice.value = if (helpful) "已记录：诊断有帮助" else "已记录反馈，我们会持续优化诊断"
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                _error.value = "反馈提交失败：${formatOperationsError(error)}"
            }
        }
    }

    fun closeIncidentDiagnosis() {
        diagnosisPhaseJob?.cancel()
        diagnosisRequestJob?.cancel()
        diagnosisPhaseJob = null
        diagnosisRequestJob = null
        _incidentDiagnosis.value = null
    }

    private suspend fun consumeIncidentDiagnosisStream(
        incidentId: Long,
        request: IncidentDiagnosisChatRequest
    ): String = withContext(Dispatchers.IO) {
        ApiClient.aiApi.incidentDiagnosisChat(incidentId, request).use { body ->
            val source = body.source()
            val answer = StringBuilder()
            val raw = StringBuilder()
            var streamError: String? = null
            var lastUiUpdate = 0L
            var lastUiLength = 0
            while (true) {
                val line = source.readUtf8Line() ?: break
                if (!line.startsWith("data:")) {
                    if (line.isNotBlank()) raw.appendLine(line)
                    continue
                }
                val payload = line.removePrefix("data:").trim()
                if (payload.isBlank() || payload == "[DONE]") continue
                try {
                    val json = JSONObject(payload)
                    when {
                        json.has("error") -> streamError = json.optString("error", "诊断服务返回错误")
                        json.has("delta") -> {
                            answer.append(json.optString("delta"))
                            val now = System.nanoTime()
                            if (lastUiUpdate == 0L || now - lastUiUpdate >= 30_000_000L || answer.length - lastUiLength >= 48) {
                                replaceIncidentDiagnosisStreaming(incidentId, answer.toString())
                                lastUiUpdate = now
                                lastUiLength = answer.length
                            }
                        }
                        json.has("reply") -> answer.append(json.optString("reply"))
                        json.has("content") -> answer.append(json.optString("content"))
                    }
                } catch (_: Exception) {
                    raw.appendLine(payload)
                }
            }
            streamError?.let { throw IOException(it) }
            if (answer.isNotBlank()) return@withContext answer.toString()
            val rawText = raw.toString().trim()
            if (rawText.isNotBlank()) {
                val parsed = try { JSONTokener(rawText).nextValue() } catch (_: Exception) { rawText }
                when (parsed) {
                    is JSONObject -> {
                        parsed.optString("error").takeIf { it.isNotBlank() }?.let { throw IOException(it) }
                        listOf("reply", "answer", "content", "message")
                            .firstNotNullOfOrNull { key -> parsed.optString(key).takeIf(String::isNotBlank) }
                            ?.let { return@withContext it }
                    }
                    is String -> return@withContext parsed
                }
            }
            throw IOException("服务器返回了空的诊断响应")
        }
    }

    private fun replaceIncidentDiagnosisStreaming(incidentId: Long, content: String) {
        updateIncidentDiagnosis(incidentId) { state ->
            val messages = state.messages.toMutableList()
            val index = messages.indexOfLast { it.role == "assistant" }
            if (index >= 0) messages[index] = messages[index].copy(content = content)
            state.copy(messages = messages)
        }
    }

    private fun updateIncidentDiagnosis(
        incidentId: Long,
        transform: (IncidentDiagnosisUiState) -> IncidentDiagnosisUiState
    ) {
        val current = _incidentDiagnosis.value ?: return
        if (current.incident.id == incidentId) _incidentDiagnosis.value = transform(current)
    }

    private fun formatOperationsError(error: Throwable): String = when (error) {
        is HttpException -> "HTTP ${error.code()}（${ApiClient.serverHost}）"
        is SocketTimeoutException -> "请求超时，请检查 AI 服务状态"
        is java.net.UnknownHostException -> "无法解析服务器域名 ${ApiClient.serverHost}"
        is java.net.ConnectException -> "无法连接服务器 ${ApiClient.serverHost}"
        else -> error.message ?: "未知错误"
    }

    fun diagnoseLog(log: StoredLog) {
        val key = "log:${log.ts}:${log.host_id}"
        if (key in _busyIds.value) return
        _busyIds.value += key
        viewModelScope.launch {
            try {
                val report = withDiagRetry("日志诊断") {
                    ApiClient.aiApi.diagnoseLog(
                        LogDiagnosisRequest(
                            host_id = log.host_id,
                            hostname = log.hostname,
                            since_min = 30,
                            single_log = "[${log.level}] ${log.hostname} ${log.message}"
                        )
                    )
                }.getOrElse { throw it }
                _diagnosis.value = report
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                _error.value = "日志诊断失败: ${error.message ?: "未知错误"}"
            } finally {
                _busyIds.value -= key
            }
        }
    }

    fun executePlaybook(id: String) {
        val key = "playbook:$id"
        if (id.isBlank() || key in _busyIds.value) return
        _busyIds.value += key
        viewModelScope.launch {
            try {
                val started = ApiClient.api.executePlaybook(id)
                check(started.ok && started.execution_id > 0) { "服务器未返回有效执行实例" }
                _notice.value = "自动化剧本已提交，正在打开执行详情"
                // 先发布一次性导航事件；执行列表刷新失败不能阻断结果页打开。
                _executionToOpen.value = started.execution_id
                try {
                    _executions.value = ApiClient.api.playbookExecutions()
                } catch (error: CancellationException) {
                    throw error
                } catch (_: Exception) {
                    // 详情页会按 execution_id 独立加载，列表稍后仍会自动刷新。
                }
                monitorExecutions()
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                _error.value = "剧本执行失败: ${error.message ?: "未知错误"}"
            } finally {
                _busyIds.value -= key
            }
        }
    }

    fun consumeExecutionNavigation() {
        _executionToOpen.value = null
    }

    fun reportExecutionNavigationFailure() {
        _error.value = "剧本已开始执行，但执行详情暂时无法打开；请从执行记录进入"
    }

    fun refreshExecutions() {
        viewModelScope.launch {
            try {
                _executions.value = ApiClient.api.playbookExecutions()
                if (_executions.value.any { it.status == "running" }) monitorExecutions()
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                _error.value = "执行记录刷新失败: ${error.message ?: "未知错误"}"
            }
        }
    }

    private fun monitorExecutions() {
        if (executionPollJob?.isActive == true) return
        executionPollJob = viewModelScope.launch {
            while (isActive) {
                delay(2_000L)
                try {
                    val latest = ApiClient.api.playbookExecutions()
                    _executions.value = latest
                    if (latest.none { it.status == "running" }) break
                } catch (error: CancellationException) {
                    throw error
                } catch (_: Exception) {
                    // 短暂网络波动不终止轮询，详情页和手动刷新仍可继续获取结果。
                }
            }
        }
    }

    fun decideRemediation(id: Long, approve: Boolean) {
        action("remediation:$id", if (approve) "已批准自动修复" else "已拒绝自动修复") {
            if (approve) ApiClient.api.approveRemediation(id) else ApiClient.api.rejectRemediation(id)
            _remediationRuns.value = ApiClient.api.remediationRuns()
            refreshOverview()
        }
    }

    fun resolveTicket(ticket: Ticket) {
        action("ticket:${ticket.id}", "工单已标记为解决") {
            // 仅发送状态变更字段，避免将可能包含 Gson 注入 null 的完整 ticket 发回后端
            val updated = ApiClient.api.updateTicket(ticket.id, mapOf("status" to "resolved"))
            _tickets.value = _tickets.value.map { if (it.id == updated.id) updated else it }
            refreshOverview()
            refreshIncidents()
        }
    }

    fun assignTicket(ticket: Ticket, assignee: String) {
        action("ticket-assign:${ticket.id}", if (assignee.isBlank()) "已取消指派" else "已指派给 $assignee") {
            val updated = ApiClient.api.updateTicket(ticket.id, mapOf("assignee" to assignee))
            _tickets.value = _tickets.value.map { if (it.id == updated.id) updated else it }
        }
    }

    fun commentTicket(ticketId: Long, text: String, attachments: List<Map<String, Any?>> = emptyList()) {
        action("ticket-comment:$ticketId", "评论已发送") {
            val body = mutableMapOf<String, Any?>("text" to text)
            if (attachments.isNotEmpty()) body["attachments"] = attachments
            val updated = ApiClient.api.commentTicket(ticketId, body)
            _tickets.value = _tickets.value.map { if (it.id == updated.id) updated else it }
        }
    }

    suspend fun loadDirectoryUsers(): List<com.aiops.monitor.data.models.DirectoryUser> =
        try { ApiClient.api.directoryUsers() } catch (_: Exception) { emptyList() }

    private fun action(
        key: String,
        successMessage: String,
        onDone: (Boolean) -> Unit = {},
        block: suspend () -> Unit
    ) {
        if (key in _busyIds.value) return
        _busyIds.value += key
        viewModelScope.launch {
            try {
                block()
                _notice.value = successMessage
                onDone(true)
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                _error.value = error.message ?: "操作失败"
                onDone(false)
            } finally {
                _busyIds.value -= key
            }
        }
    }

    private suspend fun refreshIncidents() {
        _incidents.value = ApiClient.api.incidents()
    }

    private suspend fun refreshOverview() {
        _overview.value = ApiClient.api.sreOverview()
    }

    private fun replaceIncident(updated: Incident) {
        _incidents.value = _incidents.value.map { if (it.id == updated.id) updated else it }
    }

    private suspend fun <T> apiResult(block: suspend () -> T): Result<T> = try {
        Result.success(block())
    } catch (error: CancellationException) {
        throw error
    } catch (error: Exception) {
        Result.failure(error)
    }

    fun clearNotice() { _notice.value = null }
    fun clearDiagnosis() { _diagnosis.value = null }

    // ── 端口转发 ──

    fun loadPortForwards() {
        if (_portForwardsLoading.value) return
        _portForwardsLoading.value = true
        viewModelScope.launch {
            try {
                val fwdDeferred = async { apiResult { ApiClient.api.portForwards() } }
                val hostsDeferred = async { apiResult { ApiClient.api.hosts() } }
                val hpDeferred = async { apiResult { ApiClient.api.httpProxies() } }
                fwdDeferred.await().fold(
                    onSuccess = { _portForwards.value = it },
                    onFailure = { _error.value = "端口转发加载失败: ${it.message ?: "未知错误"}" }
                )
                hostsDeferred.await().fold(
                    onSuccess = { _hosts.value = it },
                    onFailure = { /* 主机列表加载失败不影响转发展示 */ }
                )
                hpDeferred.await().fold(
                    onSuccess = { _httpProxies.value = it },
                    onFailure = { /* HTTP代理加载失败不影响转发展示 */ }
                )
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                _error.value = "端口转发加载失败: ${error.message ?: "未知错误"}"
            } finally {
                _portForwardsLoading.value = false
            }
        }
    }

    fun createPortForward(req: PortForwardCreateRequest) {
        action("port-forward:create", "端口转发规则已创建") {
            ApiClient.api.createPortForward(req)
            _portForwards.value = ApiClient.api.portForwards()
        }
    }

    fun updatePortForward(id: String, req: PortForwardEditRequest) {
        action("port-forward:$id", "端口转发规则已更新") {
            ApiClient.api.updatePortForward(id, req)
            _portForwards.value = ApiClient.api.portForwards()
        }
    }

    fun deletePortForward(id: String) {
        action("port-forward:$id", "端口转发规则已删除") {
            ApiClient.api.deletePortForward(id)
            _portForwards.value = ApiClient.api.portForwards()
        }
    }

    fun togglePortForward(id: String, enabled: Boolean) {
        action("port-forward:$id", "端口转发状态已切换") {
            ApiClient.api.togglePortForward(id, PortForwardToggleRequest(enabled))
            _portForwards.value = ApiClient.api.portForwards()
        }
    }

    fun createHTTPProxy(req: HTTPProxyConfig) {
        action("http-proxy:create", "HTTP 代理已创建") {
            ApiClient.api.createHTTPProxy(req)
            _httpProxies.value = ApiClient.api.httpProxies()
        }
    }

    fun updateHTTPProxy(id: String, req: HTTPProxyConfig) {
        action("http-proxy:$id", "HTTP 代理已更新") {
            ApiClient.api.updateHTTPProxy(id, req)
            _httpProxies.value = ApiClient.api.httpProxies()
        }
    }

    fun deleteHTTPProxy(id: String) {
        action("http-proxy:$id", "HTTP 代理已删除") {
            ApiClient.api.deleteHTTPProxy(id)
            _httpProxies.value = ApiClient.api.httpProxies()
        }
    }

    fun toggleHTTPProxy(id: String, enabled: Boolean) {
        action("http-proxy:$id", "HTTP 代理状态已切换") {
            ApiClient.api.toggleHTTPProxy(id, PortForwardToggleRequest(enabled))
            _httpProxies.value = ApiClient.api.httpProxies()
        }
    }
}

class ExecutionDetailViewModel(savedStateHandle: SavedStateHandle) : ViewModel() {
    private val executionId = savedStateHandle.get<Long>("executionId") ?: 0L
    private var requestRunning = false

    private val _execution = MutableStateFlow<PlaybookExecution?>(null)
    val execution: StateFlow<PlaybookExecution?> = _execution

    private val _loading = MutableStateFlow(false)
    val loading: StateFlow<Boolean> = _loading

    private val _error = MutableStateFlow<String?>(null)
    val error: StateFlow<String?> = _error

    private val _lastUpdated = MutableStateFlow(0L)
    val lastUpdated: StateFlow<Long> = _lastUpdated

    init {
        viewModelScope.launch {
            if (executionId <= 0) {
                _error.value = "执行实例 ID 无效"
                return@launch
            }
            while (isActive) {
                val loaded = loadOnce(showLoading = _execution.value == null)
                if (loaded && _execution.value?.status != "running") break
                delay(if (loaded) 2_000L else 5_000L)
            }
        }
    }

    fun refresh() {
        if (executionId <= 0 || _loading.value) return
        viewModelScope.launch { loadOnce(showLoading = true) }
    }

    private suspend fun loadOnce(showLoading: Boolean): Boolean {
        if (requestRunning) return false
        requestRunning = true
        if (showLoading) _loading.value = true
        return try {
            val exec = ApiClient.api.playbookExecution(executionId)
            _execution.value = exec
            _lastUpdated.value = System.currentTimeMillis()
            _error.value = null
            true
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            _error.value = error.message ?: "执行详情加载失败"
            false
        } finally {
            requestRunning = false
            _loading.value = false
        }
    }
}

/**
 * 内容审计。按主机拉取明文 HTTP 请求/响应事件，支持搜索过滤。
 */
class ContentAuditViewModel : ViewModel() {
    private val _events = MutableStateFlow<List<ContentAuditEvent>>(emptyList())
    val events: StateFlow<List<ContentAuditEvent>> = _events

    private val _loading = MutableStateFlow(false)
    val loading: StateFlow<Boolean> = _loading

    private val _error = MutableStateFlow<String?>(null)
    val error: StateFlow<String?> = _error

    fun load(hostId: String) {
        if (!ApiClient.isInitialized() || hostId.isBlank()) return
        if (_loading.value) return
        _loading.value = true
        _error.value = null
        viewModelScope.launch {
            try {
                _events.value = ApiClient.api.contentAuditEvents(hostId, 200).events.orEmpty()
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _error.value = e.message ?: "加载失败"
            } finally {
                _loading.value = false
            }
        }
    }
}

/**
 * 有数据的主机发现。用于网络相关视图的下拉过滤，默认仅列出有数据的主机。
 */
class DataHostsViewModel : ViewModel() {
    private val _netflowHosts = MutableStateFlow<Set<String>>(emptySet())
    val netflowHosts: StateFlow<Set<String>> = _netflowHosts

    private val _snmpHosts = MutableStateFlow<Set<String>>(emptySet())
    val snmpHosts: StateFlow<Set<String>> = _snmpHosts

    private val _contentAuditHosts = MutableStateFlow<Set<String>>(emptySet())
    val contentAuditHosts: StateFlow<Set<String>> = _contentAuditHosts

    private val _loading = MutableStateFlow(false)
    val loading: StateFlow<Boolean> = _loading

    fun load() {
        if (!ApiClient.isInitialized()) return
        if (_loading.value) return
        _loading.value = true
        viewModelScope.launch {
            try {
                coroutineScope {
                    val nf = async {
                        try { ApiClient.api.netflowDataHosts().hosts.orEmpty().map { it.host_id }.toSet() }
                        catch (_: Exception) { emptySet() }
                    }
                    val snmp = async {
                        try { ApiClient.api.snmpDataHosts().hosts.orEmpty().map { it.host_id }.toSet() }
                        catch (_: Exception) { emptySet() }
                    }
                    val ca = async {
                        try { ApiClient.api.contentAuditDataHosts().hosts.orEmpty().map { it.host_id }.toSet() }
                        catch (_: Exception) { emptySet() }
                    }
                    _netflowHosts.value = nf.await()
                    _snmpHosts.value = snmp.await()
                    _contentAuditHosts.value = ca.await()
                }
            } catch (_: CancellationException) {
                throw CancellationException()
            } catch (_: Exception) {
                // 静默降级
            } finally {
                _loading.value = false
            }
        }
    }
}

/** 重复主机清理：GET hosts/duplicates + POST cleanup（删除非当前且离线的旧身份）。 */
class DuplicatesViewModel : ViewModel() {
    private val _groups = MutableStateFlow<List<DupGroup>>(emptyList())
    val groups: StateFlow<List<DupGroup>> = _groups

    private val _loading = MutableStateFlow(false)
    val loading: StateFlow<Boolean> = _loading

    private val _busy = MutableStateFlow(false)
    val busy: StateFlow<Boolean> = _busy

    private val _notice = MutableStateFlow<String?>(null)
    val notice: StateFlow<String?> = _notice

    private val _error = MutableStateFlow<String?>(null)
    val error: StateFlow<String?> = _error

    fun load() {
        if (!ApiClient.isInitialized() || _loading.value) return
        _loading.value = true
        _error.value = null
        viewModelScope.launch {
            try { _groups.value = ApiClient.api.hostDuplicates().groups.orEmpty() }
            catch (e: CancellationException) { throw e }
            catch (e: Exception) { _error.value = e.message ?: "加载失败" }
            finally { _loading.value = false }
        }
    }

    fun cleanup(onDone: () -> Unit = {}) {
        if (_busy.value) return
        _busy.value = true
        viewModelScope.launch {
            try {
                val resp = ApiClient.api.cleanupDuplicates()
                val n = (resp["count"] as? Number)?.toInt() ?: 0
                _notice.value = "已清理 $n 个重复主机"
                load()
                onDone()
            } catch (e: CancellationException) { throw e }
            catch (e: Exception) { _error.value = e.message ?: "清理失败" }
            finally { _busy.value = false }
        }
    }

    fun clearNotice() { _notice.value = null }
}

/** 消息收件箱：GET messages + 标记已读/全部已读。 */
class MessagesViewModel : ViewModel() {
    private val _messages = MutableStateFlow<List<Message>>(emptyList())
    val messages: StateFlow<List<Message>> = _messages

    private val _unread = MutableStateFlow(0)
    val unread: StateFlow<Int> = _unread

    private val _loading = MutableStateFlow(false)
    val loading: StateFlow<Boolean> = _loading

    private val _error = MutableStateFlow<String?>(null)
    val error: StateFlow<String?> = _error

    fun load() {
        if (!ApiClient.isInitialized() || _loading.value) return
        _loading.value = true
        _error.value = null
        viewModelScope.launch {
            try {
                val r = ApiClient.api.messages(100)
                _messages.value = r.messages.orEmpty()
                _unread.value = r.unread
            } catch (e: CancellationException) { throw e }
            catch (e: Exception) { _error.value = e.message ?: "加载失败" }
            finally { _loading.value = false }
        }
    }

    fun markRead(ids: List<Long>) {
        if (ids.isEmpty()) return
        viewModelScope.launch {
            try {
                ApiClient.api.markMessagesRead(mapOf("ids" to ids))
                _messages.value = _messages.value.map { if (it.id in ids) it.copy(read = true) else it }
                _unread.value = _messages.value.count { !it.read }
            } catch (e: CancellationException) { throw e } catch (_: Exception) {}
        }
    }

    fun markAllRead() {
        viewModelScope.launch {
            try {
                ApiClient.api.markAllMessagesRead()
                _messages.value = _messages.value.map { it.copy(read = true) }
                _unread.value = 0
            } catch (e: CancellationException) { throw e } catch (_: Exception) {}
        }
    }
}

/** 终端会话回放：会话列表 + 单会话录制帧（需终端二次验证）。 */
class TerminalReplayViewModel : ViewModel() {
    private val _sessions = MutableStateFlow<List<TerminalSessionInfo>>(emptyList())
    val sessions: StateFlow<List<TerminalSessionInfo>> = _sessions

    private val _frames = MutableStateFlow<List<TermFrame>>(emptyList())
    val frames: StateFlow<List<TermFrame>> = _frames

    private val _loading = MutableStateFlow(false)
    val loading: StateFlow<Boolean> = _loading

    private val _error = MutableStateFlow<String?>(null)
    val error: StateFlow<String?> = _error

    // 需要终端二次验证时置真，UI 引导去设置终端密码
    private val _needTerminalVerify = MutableStateFlow(false)
    val needTerminalVerify: StateFlow<Boolean> = _needTerminalVerify

    fun loadSessions() {
        if (!ApiClient.isInitialized() || _loading.value) return
        _loading.value = true
        _error.value = null
        viewModelScope.launch {
            try { _sessions.value = ApiClient.api.terminalSessions() }
            catch (e: CancellationException) { throw e }
            catch (e: Exception) { _error.value = e.message ?: "加载失败" }
            finally { _loading.value = false }
        }
    }

    fun loadReplay(sessionId: String) {
        if (!ApiClient.isInitialized() || sessionId.isBlank()) return
        _loading.value = true
        _error.value = null
        _needTerminalVerify.value = false
        _frames.value = emptyList()
        viewModelScope.launch {
            try {
                _frames.value = ApiClient.api.terminalReplay(sessionId).frames.orEmpty()
            } catch (e: CancellationException) {
                throw e
            } catch (e: HttpException) {
                if (e.code() == 403) _needTerminalVerify.value = true
                else _error.value = "HTTP ${e.code()}"
            } catch (e: Exception) {
                _error.value = e.message ?: "加载失败"
            } finally {
                _loading.value = false
            }
        }
    }
}
