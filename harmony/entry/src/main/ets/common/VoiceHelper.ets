import { speechRecognizer, textToSpeech } from '@kit.CoreSpeechKit';
import { BusinessError } from '@kit.BasicServicesKit';

/**
 * AI 对话语音：STT（语音输入）+ TTS（朗读回复）。
 * Android AiVoiceHelper parity — CoreSpeechKit ASR + TTS.
 */
export class VoiceHelper {
  private speaking: boolean = false;
  private listening: boolean = false;
  private asrEngine: speechRecognizer.SpeechRecognitionEngine | null = null;
  private ttsEngine: textToSpeech.TextToSpeechEngine | null = null;
  private sessionId: string = '';
  private engineReady: boolean = false;
  private engineFailed: boolean = false;
  private ttsReady: boolean = false;
  private ttsFailed: boolean = false;
  private ttsInitStarted: boolean = false;

  onSpeakingChanged: ((v: boolean) => void) | null = null;
  onListeningChanged: ((v: boolean) => void) | null = null;
  onPartialResult: ((text: string) => void) | null = null;
  onFinalResult: ((text: string) => void) | null = null;
  onError: ((msg: string) => void) | null = null;

  async init(): Promise<void> {
    // Lazy ASR; kick TTS in background so 朗读 is ready when needed.
    void this.ensureTts();
  }

  isSpeaking(): boolean {
    return this.speaking;
  }

  isListening(): boolean {
    return this.listening;
  }

  speak(raw: string): void {
    const text = VoiceHelper.normalize(raw).substring(0, 1600);
    if (!text) {
      this.onError?.('暂无可朗读内容');
      return;
    }
    if (this.speaking) {
      this.stop();
      return;
    }
    void this.speakAsync(text);
  }

  private async speakAsync(text: string): Promise<void> {
    const engine = await this.ensureTts();
    if (!engine) {
      this.onError?.('语音引擎尚未就绪（请在真机验证 TTS）');
      this.speaking = false;
      this.onSpeakingChanged?.(false);
      return;
    }
    this.speaking = true;
    this.onSpeakingChanged?.(true);
    try {
      const speakParams: textToSpeech.SpeakParams = {
        requestId: `aiops-tts-${Date.now()}`
      };
      engine.speak(text, speakParams);
    } catch (e) {
      const be: BusinessError = e as BusinessError;
      this.onError?.(`朗读失败: ${be && be.message ? be.message : JSON.stringify(e)}`);
      this.speaking = false;
      this.onSpeakingChanged?.(false);
    }
  }

  stop(): void {
    try {
      this.ttsEngine?.stop();
    } catch (_e) {
      // ignore
    }
    this.speaking = false;
    this.onSpeakingChanged?.(false);
  }

  private setListening(v: boolean): void {
    if (this.listening === v) {
      return;
    }
    this.listening = v;
    this.onListeningChanged?.(v);
  }

  private wireAsrListener(engine: speechRecognizer.SpeechRecognitionEngine): void {
    const listener: speechRecognizer.RecognitionListener = {
      onStart: (_sessionId: string, _eventMessage: string): void => {
        this.setListening(true);
      },
      onEvent: (_sessionId: string, _eventCode: number, _eventMessage: string): void => {
        // no-op
      },
      onResult: (_sessionId: string, result: speechRecognizer.SpeechRecognitionResult): void => {
        const text: string = (result.result || '').trim();
        if (!text) {
          return;
        }
        if (result.isFinal || result.isLast) {
          this.onFinalResult?.(text);
        } else {
          this.onPartialResult?.(text);
        }
      },
      onComplete: (_sessionId: string, _eventMessage: string): void => {
        this.setListening(false);
      },
      onError: (_sessionId: string, errorCode: number, errorMessage: string): void => {
        this.setListening(false);
        // Client cancel / no-match style noise — keep quiet when user stops.
        if (errorCode === 1002200004) {
          return;
        }
        const msg: string = errorMessage && errorMessage.length > 0 ?
          errorMessage : `语音识别失败（${errorCode}）`;
        this.onError?.(msg);
      }
    };
    engine.setListener(listener);
  }

  private wireTtsListener(engine: textToSpeech.TextToSpeechEngine): void {
    const listener: textToSpeech.SpeakListener = {
      onStart: (_requestId: string, _response: textToSpeech.StartResponse): void => {
        this.speaking = true;
        this.onSpeakingChanged?.(true);
      },
      onComplete: (_requestId: string, _response: textToSpeech.CompleteResponse): void => {
        this.speaking = false;
        this.onSpeakingChanged?.(false);
      },
      onStop: (_requestId: string, _response: textToSpeech.StopResponse): void => {
        this.speaking = false;
        this.onSpeakingChanged?.(false);
      },
      onError: (_requestId: string, errorCode: number, errorMessage: string): void => {
        this.speaking = false;
        this.onSpeakingChanged?.(false);
        const msg: string = errorMessage && errorMessage.length > 0 ?
          errorMessage : `朗读失败（${errorCode}）`;
        this.onError?.(msg);
      }
    };
    engine.setListener(listener);
  }

  private async ensureTts(): Promise<textToSpeech.TextToSpeechEngine | null> {
    if (this.ttsFailed) {
      return null;
    }
    if (this.ttsEngine && this.ttsReady) {
      return this.ttsEngine;
    }
    if (this.ttsInitStarted && !this.ttsReady) {
      // Still initializing — avoid parallel createEngine storms.
      return null;
    }
    this.ttsInitStarted = true;
    try {
      const params: textToSpeech.CreateEngineParams = {
        language: 'zh-CN',
        person: 0,
        online: 1
      };
      const engine: textToSpeech.TextToSpeechEngine = await textToSpeech.createEngine(params);
      this.ttsEngine = engine;
      this.ttsReady = true;
      this.wireTtsListener(engine);
      return engine;
    } catch (e) {
      this.ttsFailed = true;
      this.ttsEngine = null;
      this.ttsReady = false;
      const be: BusinessError = e as BusinessError;
      const detail: string = be && be.message ? be.message : '当前设备不支持语音朗读';
      this.onError?.(detail);
      return null;
    }
  }

  private async ensureAsr(): Promise<speechRecognizer.SpeechRecognitionEngine | null> {
    if (this.asrEngine && this.engineReady) {
      return this.asrEngine;
    }
    try {
      const extraParams: Record<string, Object> = {};
      extraParams['locate'] = 'CN';
      extraParams['recognizerMode'] = 'short';
      const initParams: speechRecognizer.CreateEngineParams = {
        language: 'zh-CN',
        online: 1,
        extraParams: extraParams
      };
      const engine: speechRecognizer.SpeechRecognitionEngine =
        await speechRecognizer.createEngine(initParams);
      this.asrEngine = engine;
      this.engineReady = true;
      this.engineFailed = false;
      this.wireAsrListener(engine);
      return engine;
    } catch (e) {
      this.engineFailed = true;
      this.asrEngine = null;
      this.engineReady = false;
      const be: BusinessError = e as BusinessError;
      const detail: string = be && be.message ? be.message : '当前设备不支持语音识别';
      this.onError?.(detail);
      return null;
    }
  }

  async startListening(): Promise<void> {
    // Abort any prior session cleanly, then allow a fresh engine attempt.
    await this.stopListening(true);
    this.engineFailed = false;
    const engine: speechRecognizer.SpeechRecognitionEngine | null = await this.ensureAsr();
    if (!engine) {
      if (!this.engineFailed) {
        this.onError?.('当前设备不支持语音识别');
      }
      return;
    }
    this.sessionId = `aiops-asr-${Date.now()}`;
    const audioInfo: speechRecognizer.AudioInfo = {
      audioType: 'pcm',
      sampleRate: 16000,
      soundChannel: 1,
      sampleBit: 16
    };
    const extraParam: Record<string, Object> = {};
    extraParam['recognitionMode'] = 0;
    extraParam['vadBegin'] = 2000;
    extraParam['vadEnd'] = 3000;
    extraParam['maxAudioDuration'] = 20000;
    const startParams: speechRecognizer.StartParams = {
      sessionId: this.sessionId,
      audioInfo: audioInfo,
      extraParams: extraParam
    };
    try {
      engine.startListening(startParams);
      this.setListening(true);
    } catch (e) {
      this.setListening(false);
      const be: BusinessError = e as BusinessError;
      this.onError?.(be && be.message ? be.message : '无法启动语音输入');
    }
  }

  /**
   * @param abort When true, cancel (discard finals). When false, finish (flush final result).
   */
  async stopListening(abort: boolean = false): Promise<void> {
    const engine: speechRecognizer.SpeechRecognitionEngine | null = this.asrEngine;
    const sid: string = this.sessionId;
    if (engine && sid) {
      try {
        if (abort) {
          engine.cancel(sid);
        } else {
          engine.finish(sid);
        }
      } catch (_e) {
        // ignore
      }
    }
    this.sessionId = '';
    this.setListening(false);
  }

  release(): void {
    void this.stopListening(true);
    this.stop();
    if (this.asrEngine) {
      try {
        this.asrEngine.shutdown();
      } catch (_e) {
        // ignore
      }
    }
    if (this.ttsEngine) {
      try {
        this.ttsEngine.shutdown();
      } catch (_e2) {
        // ignore
      }
    }
    this.asrEngine = null;
    this.ttsEngine = null;
    this.engineReady = false;
    this.ttsReady = false;
  }

  static normalize(raw: string): string {
    return (raw || '')
      .replace(/```[\s\S]*?```/g, ' ')
      .replace(/`[^`]+`/g, ' ')
      .replace(/[#>*_~|]/g, ' ')
      .replace(/\s+/g, ' ')
      .trim();
  }
}
