package com.aiops.monitor.ui

import android.content.Context
import android.content.Intent
import android.os.Bundle
import android.speech.RecognitionListener
import android.speech.RecognizerIntent
import android.speech.SpeechRecognizer
import android.speech.tts.TextToSpeech
import android.speech.tts.UtteranceProgressListener
import java.util.Locale

/**
 * AI 对话语音：STT（语音输入）+ TTS（朗读回复）。
 * 优先选用成熟稳重的中文女声（排除男声），语速略慢、音调略低。
 */
class AiVoiceHelper(context: Context) {
    private val appCtx = context.applicationContext
    private var tts: TextToSpeech? = null
    private var ttsReady = false
    private var speaking = false
    private var recognizer: SpeechRecognizer? = null
    private var listening = false

    var onListeningChanged: ((Boolean) -> Unit)? = null
    var onPartialResult: ((String) -> Unit)? = null
    var onFinalResult: ((String) -> Unit)? = null
    var onSpeakingChanged: ((Boolean) -> Unit)? = null
    var onError: ((String) -> Unit)? = null

    init {
        tts = TextToSpeech(appCtx) { status ->
            ttsReady = status == TextToSpeech.SUCCESS
            if (ttsReady) configureVoice()
        }
        tts?.setOnUtteranceProgressListener(object : UtteranceProgressListener() {
            override fun onStart(utteranceId: String?) {
                speaking = true
                onSpeakingChanged?.invoke(true)
            }
            override fun onDone(utteranceId: String?) {
                speaking = false
                onSpeakingChanged?.invoke(false)
            }
            @Deprecated("Deprecated in Java")
            override fun onError(utteranceId: String?) {
                speaking = false
                onSpeakingChanged?.invoke(false)
            }
        })
    }

    private fun configureVoice() {
        val engine = tts ?: return
        engine.language = Locale.SIMPLIFIED_CHINESE
        // 成熟稳重：略慢 + 音调略低（偏高易显幼声）
        engine.setSpeechRate(0.88f)
        engine.setPitch(0.96f)
        val voices = try { engine.voices } catch (_: Exception) { null } ?: return
        // 成熟稳重女声优先；幼声名靠后，男声明确排除
        val mature = listOf(
            "xiaoxuan", "xiaohan", "huihui", "xiaoyi", "xiaoqiu", "xiaorou", "xiaoyan", "xiaoshuang"
        )
        val female = listOf("xiaoxiao", "xiaochen", "xiaomeng", "female", "女", "woman")
        val quality = listOf("neural", "natural", "premium", "enhanced")
        val maleHints = listOf(
            "yunyang", "yunxi", "yunjian", "yunhao", "yunfeng", "kangkang", "male", "男"
        )
        fun label(v: android.speech.tts.Voice): String =
            ((v.name ?: "") + " " + (v.locale?.toLanguageTag() ?: "")).lowercase()
        fun isMale(v: android.speech.tts.Voice) = maleHints.any { label(v).contains(it) }
        fun isZh(v: android.speech.tts.Voice): Boolean {
            val lang = v.locale?.toLanguageTag()?.lowercase().orEmpty()
            return lang.startsWith("zh") || lang.contains("cmn") || lang.contains("chinese")
        }
        val zh = voices.filter(::isZh)
        val pool = (zh.ifEmpty { voices.toList() }).filter { !isMale(it) }
        val best = pool.firstOrNull { v -> mature.any { label(v).contains(it) } }
            ?: pool.filter { v -> female.any { label(v).contains(it) } }
                .maxByOrNull { v ->
                    var score = v.quality
                    if (quality.any { label(v).contains(it) }) score += 100
                    score
                }
            ?: pool.maxByOrNull { it.quality }
            ?: pool.firstOrNull()
            ?: zh.firstOrNull { !isMale(it) }
            ?: zh.firstOrNull()
        if (best != null) {
            try { engine.voice = best } catch (_: Exception) { /* keep default */ }
        }
    }

    fun isSpeaking(): Boolean = speaking
    fun isListening(): Boolean = listening

    fun speak(text: String) {
        val clean = text
            .replace(Regex("```[\\s\\S]*?```"), " ")
            .replace(Regex("`[^`]+`"), " ")
            .replace(Regex("[#>*_~|]"), " ")
            .replace(Regex("\\s+"), " ")
            .trim()
            .take(1600)
        if (clean.isBlank()) {
            onError?.invoke("暂无可朗读内容")
            return
        }
        if (!ttsReady) {
            onError?.invoke("语音引擎尚未就绪")
            return
        }
        if (speaking) {
            stopSpeaking()
            return
        }
        tts?.speak(clean, TextToSpeech.QUEUE_FLUSH, null, "aiops-ai-${System.currentTimeMillis()}")
    }

    fun stopSpeaking() {
        tts?.stop()
        speaking = false
        onSpeakingChanged?.invoke(false)
    }

    fun startListening() {
        if (!SpeechRecognizer.isRecognitionAvailable(appCtx)) {
            onError?.invoke("当前设备不支持语音识别")
            return
        }
        stopListening()
        val sr = SpeechRecognizer.createSpeechRecognizer(appCtx)
        recognizer = sr
        sr.setRecognitionListener(object : RecognitionListener {
            override fun onReadyForSpeech(params: Bundle?) {
                listening = true
                onListeningChanged?.invoke(true)
            }
            override fun onBeginningOfSpeech() {}
            override fun onRmsChanged(rmsdB: Float) {}
            override fun onBufferReceived(buffer: ByteArray?) {}
            override fun onEndOfSpeech() {}
            override fun onError(error: Int) {
                listening = false
                onListeningChanged?.invoke(false)
                if (error != SpeechRecognizer.ERROR_CLIENT && error != SpeechRecognizer.ERROR_NO_MATCH) {
                    onError?.invoke("语音识别失败（$error）")
                }
            }
            override fun onResults(results: Bundle?) {
                listening = false
                onListeningChanged?.invoke(false)
                val text = results
                    ?.getStringArrayList(SpeechRecognizer.RESULTS_RECOGNITION)
                    ?.firstOrNull()
                    ?.trim()
                    .orEmpty()
                if (text.isNotBlank()) onFinalResult?.invoke(text)
            }
            override fun onPartialResults(partialResults: Bundle?) {
                val text = partialResults
                    ?.getStringArrayList(SpeechRecognizer.RESULTS_RECOGNITION)
                    ?.firstOrNull()
                    ?.trim()
                    .orEmpty()
                if (text.isNotBlank()) onPartialResult?.invoke(text)
            }
            override fun onEvent(eventType: Int, params: Bundle?) {}
        })
        val intent = Intent(RecognizerIntent.ACTION_RECOGNIZE_SPEECH).apply {
            putExtra(RecognizerIntent.EXTRA_LANGUAGE_MODEL, RecognizerIntent.LANGUAGE_MODEL_FREE_FORM)
            putExtra(RecognizerIntent.EXTRA_LANGUAGE, "zh-CN")
            putExtra(RecognizerIntent.EXTRA_PARTIAL_RESULTS, true)
            putExtra(RecognizerIntent.EXTRA_MAX_RESULTS, 1)
        }
        try {
            sr.startListening(intent)
        } catch (e: Exception) {
            listening = false
            onListeningChanged?.invoke(false)
            onError?.invoke(e.message ?: "无法启动语音输入")
        }
    }

    fun stopListening() {
        try { recognizer?.stopListening() } catch (_: Exception) {}
        try { recognizer?.cancel() } catch (_: Exception) {}
        try { recognizer?.destroy() } catch (_: Exception) {}
        recognizer = null
        if (listening) {
            listening = false
            onListeningChanged?.invoke(false)
        }
    }

    fun release() {
        stopListening()
        stopSpeaking()
        try { tts?.shutdown() } catch (_: Exception) {}
        tts = null
        ttsReady = false
    }
}
