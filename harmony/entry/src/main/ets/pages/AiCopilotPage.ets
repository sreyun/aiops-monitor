import { abilityAccessCtrl, common, Permissions } from '@kit.AbilityKit';
import { photoAccessHelper } from '@kit.MediaLibraryKit';
import { fileIo as fs, picker } from '@kit.CoreFileKit';
import { util } from '@kit.ArkTS';
import { Api } from '../data/Api';
import { ApiClient, StreamUiThrottle } from '../data/ApiClient';
import { AppTheme } from '../common/Routes';
import {
  ChatMessage,
  ObservabilityDataSource,
  AiChatRequest,
  AiHistoryMessage,
  AiImagePart,
  AiFilePart,
  PendingAiAttachment,
  HermesParseResponse,
  AiUiAction
} from '../data/models/Models';
import { ErrorBanner, PageHeader } from '../components/CommonUI';
import { ToastHost } from '../components/InteractionUI';
import { MarkdownText } from '../components/MarkdownUI';
import { VoiceHelper } from '../common/VoiceHelper';
import { NavUtil } from '../common/NavUtil';
import { AppSymbol } from '../components/AppIcons';

const MAX_ATTACH_BYTES: number = 8 * 1024 * 1024;

@Component
export struct AiCopilotTab {
  @StorageLink('aiops_theme_rev') themeRev: number = 0;

  @StorageLink('aiops_bg') tBg: string = '#080C14';
  @StorageLink('aiops_card') tCard: string = '#111722';
  @StorageLink('aiops_elevated') tElevated: string = '#192130';
  @StorageLink('aiops_text') tText: string = '#E6E8F0';
  @StorageLink('aiops_muted') tMuted: string = '#9AA3B8';
  @StorageLink('aiops_accent') tAccent: string = '#4F7FFF';
  @StorageLink('aiops_accent_soft') tAccentSoft: string = '#1E3166';
  @StorageLink('aiops_accent_on') tAccentOn: string = '#D6E2FF';
  @StorageLink('aiops_success') tSuccess: string = '#00A86B';
  @StorageLink('aiops_success_soft') tSuccessSoft: string = '#0F3A2C';
  @StorageLink('aiops_warning') tWarning: string = '#F59E0B';
  @StorageLink('aiops_warning_soft') tWarningSoft: string = '#3A2A10';
  @StorageLink('aiops_danger') tDanger: string = '#EF4444';
  @StorageLink('aiops_danger_soft') tDangerSoft: string = '#3A1614';
  @StorageLink('aiops_border') tBorder: string = '#39445A';
  @StorageLink('aiops_border_soft') tBorderSoft: string = '#222C3D';
  @StorageLink('aiops_on_accent') tOnAccent: string = '#FFFFFF';
  @StorageLink('aiops_nav_bar') tNavBar: string = '#111722';
  @StorageLink('aiops_scrim') tScrim: string = '#99000000';
  @StorageLink('aiops_shadow') tShadow: string = '#40000000';

  @State input: string = '';
  @State messages: ChatMessage[] = [];
  @State suggestions: string[] = [];
  @State caps: string[] = ['知识库检索', 'MCP 工具'];
  @State streaming: boolean = false;
  @State activity: string = '';
  @State error: string = '';
  @State speaking: boolean = false;
  @State listening: boolean = false;
  @State toastMsg: string = '';
  @State toastError: boolean = false;
  @State pending: PendingAiAttachment[] = [];
  @State attaching: boolean = false;
  @State sttPartial: string = '';
  private voice: VoiceHelper = new VoiceHelper();
  private hostId: string = '';
  private cancelStream: boolean = false;
  private sessionId: number = 0;
  private streamThrottle: StreamUiThrottle | null = null;
  private listScroller: Scroller = new Scroller();
  private lastRetryText: string = '';
  private sttBaseInput: string = '';

  private scrollToEnd(): void {
    const last = this.messages.length - 1;
    if (last < 0) {
      return;
    }
    try {
      this.listScroller.scrollToIndex(last, true, ScrollAlign.END);
    } catch (_e) {
      // ignore
    }
  }

  aboutToAppear(): void {
    void this.onAppearAsync();
  }

  private async onAppearAsync(): Promise<void> {
    this.hostId = NavUtil.paramString('hostId', '');
    const prompt = NavUtil.paramString('prompt', '');
    const display = NavUtil.paramString('display', '');
    if (prompt) {
      this.input = display || prompt;
    } else if (this.hostId) {
      this.input = `请诊断主机 ${this.hostId} 的当前问题`;
    }
    await this.voice.init();
    this.voice.onSpeakingChanged = (v: boolean) => {
      this.speaking = v;
    };
    this.voice.onListeningChanged = (v: boolean) => {
      this.listening = v;
      if (!v) {
        this.sttPartial = '';
      }
    };
    this.voice.onPartialResult = (text: string) => {
      const t: string = (text || '').trim();
      this.sttPartial = t;
      if (t.length === 0) {
        return;
      }
      this.input = this.sttBaseInput.trim().length === 0 ? t : `${this.sttBaseInput} ${t}`;
    };
    this.voice.onFinalResult = (text: string) => {
      const t: string = (text || '').trim();
      this.sttPartial = '';
      if (!t) {
        return;
      }
      this.input = this.sttBaseInput.trim().length === 0 ? t : `${this.sttBaseInput} ${t}`;
      this.sttBaseInput = this.input;
    };
    this.voice.onError = (m: string) => {
      this.toast(m, true);
    };
    try {
      const s = await Api.aiSuggestions();
      const dyn = s.dynamic ?? [];
      const cur = s.curated ?? [];
      const merged: string[] = [];
      for (let i = 0; i < dyn.length && merged.length < 8; i++) {
        merged.push(dyn[i]);
      }
      for (let i = 0; i < cur.length && merged.length < 8; i++) {
        merged.push(cur[i]);
      }
      this.suggestions = merged;
    } catch (_e) {
      // optional
    }
    try {
      const ds = await Api.datasources();
      this.caps = this.buildCaps(ds ?? []);
    } catch (_e2) {
      this.caps = ['知识库检索', 'MCP 工具', 'Loki 0', 'Prometheus 0'];
    }
    if (prompt) {
      void this.sendText(display || prompt, prompt);
    }
  }

  aboutToDisappear(): void {
    this.cancelStream = true;
    ApiClient.cancelActiveSse();
    this.voice.release();
  }

  private buildCaps(list: ObservabilityDataSource[]): string[] {
    let loki = 0;
    let prom = 0;
    for (let i = 0; i < list.length; i++) {
      const t = (list[i].type || '').toLowerCase();
      if (t.indexOf('loki') >= 0 || t.indexOf('log') >= 0) {
        loki++;
      }
      if (t.indexOf('prom') >= 0 || t.indexOf('metric') >= 0 || t.indexOf('victoria') >= 0) {
        prom++;
      }
    }
    return ['知识库检索', 'MCP 工具', `Loki ${loki}`, `Prometheus ${prom}`];
  }

  private toast(msg: string, err: boolean = false): void {
    this.toastMsg = msg;
    this.toastError = err;
    setTimeout(() => {
      if (this.toastMsg === msg) {
        this.toastMsg = '';
      }
    }, 2200);
  }

  private clearChat(): void {
    this.messages = [];
    this.pending = [];
    this.sessionId = 0;
    this.activity = '';
    this.toast('已清空对话');
  }

  private stopGen(): void {
    this.cancelStream = true;
    this.streaming = false;
    this.activity = '';
    if (this.streamThrottle) {
      this.streamThrottle.flush();
      this.streamThrottle = null;
    }
    ApiClient.cancelActiveSse();
    this.toast('已停止生成');
  }

  private replaceStreaming(content: string, type: string = 'streaming', actions?: AiUiAction[]): void {
    const list = this.messages.slice();
    let idx = -1;
    for (let i = list.length - 1; i >= 0; i--) {
      if (list[i].role === 'assistant' && (list[i].type === 'streaming' || type !== 'streaming')) {
        if (list[i].type === 'streaming' || i === list.length - 1) {
          idx = i;
          break;
        }
      }
    }
    if (idx < 0) {
      for (let i = list.length - 1; i >= 0; i--) {
        if (list[i].role === 'assistant') {
          idx = i;
          break;
        }
      }
    }
    if (idx >= 0) {
      const prev = list[idx];
      list[idx] = {
        role: 'assistant',
        content: content,
        type: type,
        actions: actions !== undefined ? actions : (prev.actions || [])
      };
    } else {
      list.push({ role: 'assistant', content: content, type: type, actions: actions || [] });
    }
    this.messages = list;
  }

  private appendStreamingAction(act: AiUiAction): void {
    const list = this.messages.slice();
    let idx = -1;
    for (let i = list.length - 1; i >= 0; i--) {
      if (list[i].role === 'assistant' && list[i].type === 'streaming') {
        idx = i;
        break;
      }
    }
    if (idx < 0) {
      return;
    }
    const prev = list[idx];
    const cur = (prev.actions || []).slice();
    const dup = cur.some((a: AiUiAction) => a.type === act.type && (a.id || '') === (act.id || '') && (a.label || '') === (act.label || ''));
    if (dup) {
      return;
    }
    cur.push(act);
    list[idx] = { role: prev.role, content: prev.content, type: prev.type, actions: cur };
    this.messages = list;
  }

  private handleUiAction(act: AiUiAction): void {
    if (!act || !act.type) {
      return;
    }
    if (act.type === 'open_dashboard' && act.id) {
      NavUtil.push('pages/DashboardViewPage', { dashboardId: act.id });
      this.toast('已打开看板');
      return;
    }
    if (act.type === 'export_report') {
      this.toast('完整导出请使用 Web 端；可先复制正文');
      return;
    }
    if (act.type === 'show_chart' || act.type === 'show_stat') {
      let tip = act.title || act.label || '指标卡片';
      if (act.value !== undefined && act.value !== null) {
        tip += ` · ${act.value}${act.unit || ''}`;
      }
      tip += '（完整交互图表请用 Web 端）';
      this.toast(tip);
      return;
    }
    if (act.type === 'drill_down') {
      const target = act.target || '';
      if (target === 'host_detail' && act.host_id) {
        NavUtil.push('pages/HostDetailPage', { hostId: act.host_id });
        this.toast('已打开主机详情');
        return;
      }
      if (target === 'dashboard' && act.id) {
        NavUtil.push('pages/DashboardViewPage', { dashboardId: act.id });
        this.toast('已打开看板');
        return;
      }
      if ((target === 'prompt' || act.prompt) && act.prompt) {
        this.input = act.prompt;
        void this.sendText(act.prompt);
        return;
      }
    }
    this.toast(act.label || act.type);
  }

  private buildHistory(): AiHistoryMessage[] {
    const out: AiHistoryMessage[] = [];
    for (let i = 0; i < this.messages.length; i++) {
      const m = this.messages[i];
      if (m.role !== 'user' && m.role !== 'assistant') {
        continue;
      }
      if (m.type === 'streaming' || m.type === 'error') {
        continue;
      }
      if (!(m.content || '').trim()) {
        continue;
      }
      out.push({ role: m.role, content: m.content });
    }
    if (out.length > 10) {
      return out.slice(out.length - 10);
    }
    return out;
  }

  private fileNameFromUri(uri: string, fallback: string): string {
    const slash: number = uri.lastIndexOf('/');
    const raw: string = slash >= 0 ? uri.substring(slash + 1) : uri;
    const q: number = raw.indexOf('?');
    const name: string = q >= 0 ? raw.substring(0, q) : raw;
    try {
      return decodeURIComponent(name) || fallback;
    } catch (_e) {
      return name.length > 0 ? name : fallback;
    }
  }

  private guessMime(name: string, fallback: string): string {
    const lower: string = name.toLowerCase();
    if (lower.endsWith('.png')) {
      return 'image/png';
    }
    if (lower.endsWith('.jpg') || lower.endsWith('.jpeg')) {
      return 'image/jpeg';
    }
    if (lower.endsWith('.gif')) {
      return 'image/gif';
    }
    if (lower.endsWith('.webp')) {
      return 'image/webp';
    }
    if (lower.endsWith('.bmp')) {
      return 'image/bmp';
    }
    if (lower.endsWith('.pdf')) {
      return 'application/pdf';
    }
    if (lower.endsWith('.json')) {
      return 'application/json';
    }
    if (lower.endsWith('.txt') || lower.endsWith('.log') || lower.endsWith('.md')) {
      return 'text/plain';
    }
    if (lower.endsWith('.doc')) {
      return 'application/msword';
    }
    if (lower.endsWith('.docx')) {
      return 'application/vnd.openxmlformats-officedocument.wordprocessingml.document';
    }
    if (lower.endsWith('.xls')) {
      return 'application/vnd.ms-excel';
    }
    if (lower.endsWith('.xlsx')) {
      return 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet';
    }
    return fallback;
  }

  private readUriBytes(uri: string): Uint8Array | null {
    let file: fs.File | undefined = undefined;
    try {
      file = fs.openSync(uri, fs.OpenMode.READ_ONLY);
      // Prefer open+read capped at MAX — Photo/Document URIs often report size=0 via stat.
      try {
        const st = fs.statSync(file.fd);
        if (st.size > MAX_ATTACH_BYTES) {
          return null;
        }
      } catch (_st) {
        // ignore missing/zero size; still attempt read
      }
      const buf: ArrayBuffer = new ArrayBuffer(MAX_ATTACH_BYTES + 1);
      const readLen: number = fs.readSync(file.fd, buf);
      if (readLen <= 0) {
        return null;
      }
      if (readLen > MAX_ATTACH_BYTES) {
        // Oversized — reject to match Android 8MB cap.
        return null;
      }
      return new Uint8Array(buf, 0, readLen);
    } catch (_e) {
      return null;
    } finally {
      if (file !== undefined) {
        try {
          fs.closeSync(file);
        } catch (_c) {
          // ignore
        }
      }
    }
  }

  private bytesToUtf8(bytes: Uint8Array): string {
    const decoder: util.TextDecoder = util.TextDecoder.create('utf-8', { ignoreBOM: true });
    return decoder.decodeToString(bytes);
  }

  private bytesToBase64(bytes: Uint8Array): string {
    const helper: util.Base64Helper = new util.Base64Helper();
    return helper.encodeToStringSync(bytes);
  }

  private isImageName(name: string, mime: string): boolean {
    if (mime.indexOf('image/') === 0) {
      return true;
    }
    return /\.(png|jpe?g|gif|webp|bmp)$/i.test(name);
  }

  private isTextExt(name: string, mime: string): boolean {
    if (mime.indexOf('text/') === 0) {
      return true;
    }
    const lower: string = name.toLowerCase();
    return lower.endsWith('.txt') || lower.endsWith('.log') || lower.endsWith('.md') ||
      lower.endsWith('.json') || lower.endsWith('.yaml') || lower.endsWith('.yml') ||
      lower.endsWith('.csv') || lower.endsWith('.xml') || lower.endsWith('.sh') ||
      lower.endsWith('.py') || lower.endsWith('.go') || lower.endsWith('.js') ||
      lower.endsWith('.ts') || lower.endsWith('.conf') || lower.endsWith('.cfg') ||
      lower.endsWith('.ini');
  }

  private isBinaryDoc(name: string): boolean {
    const lower: string = name.toLowerCase();
    return lower.endsWith('.pdf') || lower.endsWith('.doc') || lower.endsWith('.docx') ||
      lower.endsWith('.xls') || lower.endsWith('.xlsx');
  }

  private async prepareAiAttachments(uris: string[], forceImage: boolean): Promise<PendingAiAttachment[]> {
    const out: PendingAiAttachment[] = [];
    let imageCount: number = 0;
    for (let i = 0; i < this.pending.length; i++) {
      if (this.pending[i].kind === 'image') {
        imageCount++;
      }
    }
    for (let i = 0; i < uris.length; i++) {
      const uri: string = uris[i];
      const name: string = this.fileNameFromUri(uri, forceImage ? `image-${i + 1}` : `file-${i + 1}`);
      const mime: string = this.guessMime(name, forceImage ? 'image/jpeg' : 'application/octet-stream');
      const bytes: Uint8Array | null = this.readUriBytes(uri);
      if (!bytes) {
        continue;
      }
      if (forceImage || this.isImageName(name, mime)) {
        if (imageCount >= 4) {
          continue;
        }
        out.push({
          name: name,
          mime: mime.indexOf('image/') === 0 ? mime : 'image/jpeg',
          kind: 'image',
          data: this.bytesToBase64(bytes),
          text: ''
        });
        imageCount++;
        continue;
      }
      if (this.isTextExt(name, mime)) {
        out.push({
          name: name,
          mime: mime,
          kind: 'file',
          data: '',
          text: this.bytesToUtf8(bytes)
        });
        continue;
      }
      if (this.isBinaryDoc(name)) {
        try {
          const b64: string = this.bytesToBase64(bytes);
          const body: Record<string, string> = {
            'name': name,
            'mime': mime,
            'data': b64
          };
          const resp: HermesParseResponse = await Api.hermesParse(body);
          const parsedText: string = resp.text ?? resp.content ?? '';
          if (parsedText.trim().length > 0) {
            out.push({ name: name, mime: mime, kind: 'file', data: '', text: parsedText });
          }
        } catch (_e) {
          // skip unreadable binary
        }
      }
    }
    return out;
  }

  private showAttachSheet(): void {
    if (this.attaching || this.streaming) {
      return;
    }
    try {
      this.getUIContext().showActionSheet({
        title: '添加附件',
        message: '选择图片或文档',
        autoCancel: true,
        sheets: [
          {
            title: '图片',
            action: (): void => {
              void this.pickImages();
            }
          },
          {
            title: '文档 (txt/json/pdf/doc/xls…)',
            action: (): void => {
              void this.pickDocuments();
            }
          }
        ],
        cancel: (): void => {
          // dismissed
        }
      });
    } catch (_e) {
      // Fallback: open image picker directly
      void this.pickImages();
    }
  }

  private async pickImages(): Promise<void> {
    if (this.attaching || this.streaming) {
      return;
    }
    this.attaching = true;
    try {
      const photoPicker: photoAccessHelper.PhotoViewPicker = new photoAccessHelper.PhotoViewPicker();
      const result: photoAccessHelper.PhotoSelectResult = await photoPicker.select({
        MIMEType: photoAccessHelper.PhotoViewMIMETypes.IMAGE_TYPE,
        maxSelectNumber: 3
      });
      const uris: string[] = result.photoUris ?? [];
      if (uris.length === 0) {
        this.toast('未选择附件');
        return;
      }
      const added: PendingAiAttachment[] = await this.prepareAiAttachments(uris, true);
      if (added.length === 0) {
        this.toast('附件读取失败或超过 8MB', true);
        return;
      }
      this.pending = this.pending.concat(added);
      this.toast(`已添加 ${added.length} 个附件`);
    } catch (e) {
      this.toast(`附件选择失败: ${ApiClient.formatError(e as Object)}`, true);
    } finally {
      this.attaching = false;
    }
  }

  private async pickDocuments(): Promise<void> {
    if (this.attaching || this.streaming) {
      return;
    }
    this.attaching = true;
    try {
      const ctx: common.UIAbilityContext = getContext(this) as common.UIAbilityContext;
      const options: picker.DocumentSelectOptions = new picker.DocumentSelectOptions();
      options.maxSelectNumber = 5;
      options.fileSuffixFilters = [
        '文档|.txt,.log,.md,.json,.csv,.xml,.pdf,.doc,.docx,.xls,.xlsx',
        '图片|.png,.jpg,.jpeg,.gif,.webp,.bmp'
      ];
      const documentPicker: picker.DocumentViewPicker = new picker.DocumentViewPicker(ctx);
      const uris: Array<string> = await documentPicker.select(options);
      if (!uris || uris.length === 0) {
        this.toast('未选择附件');
        return;
      }
      const added: PendingAiAttachment[] = await this.prepareAiAttachments(uris, false);
      if (added.length === 0) {
        this.toast('附件读取失败、超过 8MB 或格式不支持', true);
        return;
      }
      this.pending = this.pending.concat(added);
      this.toast(`已添加 ${added.length} 个附件`);
    } catch (e) {
      this.toast(`附件选择失败: ${ApiClient.formatError(e as Object)}`, true);
    } finally {
      this.attaching = false;
    }
  }

  private async ensureMicThenListen(): Promise<void> {
    try {
      const ctx: common.UIAbilityContext = getContext(this) as common.UIAbilityContext;
      const atManager = abilityAccessCtrl.createAtManager();
      const perms: Permissions[] = ['ohos.permission.MICROPHONE'];
      const grant = await atManager.requestPermissionsFromUser(ctx, perms);
      if (grant.authResults.length > 0 && grant.authResults[0] !== 0) {
        this.toast('需要麦克风权限才能语音输入', true);
        return;
      }
      await this.voice.startListening();
    } catch (e) {
      this.toast(`语音识别失败: ${ApiClient.formatError(e as Object)}`, true);
    }
  }

  private onMic(): void {
    if (this.streaming) {
      return;
    }
    if (this.listening) {
      void this.voice.stopListening(false);
      return;
    }
    this.sttBaseInput = this.input;
    this.sttPartial = '';
    void this.ensureMicThenListen();
  }

  private removePending(index: number): void {
    const next: PendingAiAttachment[] = this.pending.slice();
    if (index < 0 || index >= next.length) {
      return;
    }
    next.splice(index, 1);
    this.pending = next;
  }

  async sendText(displayText: string, requestText?: string): Promise<void> {
    const shown = displayText.trim();
    let msg = (requestText ?? displayText).trim();
    const pendingSnap: PendingAiAttachment[] = this.pending.slice();
    if ((!msg && pendingSnap.length === 0) || this.streaming) {
      return;
    }
    if (!msg) {
      msg = '请分析附件内容';
    }
    this.input = '';
    const images: AiImagePart[] = [];
    const files: AiFilePart[] = [];
    for (let i = 0; i < pendingSnap.length; i++) {
      const p: PendingAiAttachment = pendingSnap[i];
      if (p.kind === 'image') {
        images.push({ mime: p.mime, data: p.data });
      } else {
        files.push({ name: p.name, text: p.text });
      }
    }
    const names: string[] = pendingSnap.map((p: PendingAiAttachment) => p.name);
    let attachNote: string = '';
    if (images.length > 0) {
      attachNote += `\n[图片 ×${images.length}]`;
    }
    if (files.length > 0) {
      attachNote += `\n[附件: ${names.filter((_n: string, idx: number) => pendingSnap[idx].kind === 'file').join(', ')}]`;
    }
    this.pending = [];
    const history = this.buildHistory();
    const withUser = this.messages.slice();
    withUser.push({ role: 'user', content: (shown || msg) + attachNote });
    withUser.push({ role: 'assistant', content: '', type: 'streaming' });
    this.messages = withUser;
    this.streaming = true;
    this.cancelStream = false;
    this.error = '';
    this.activity = '正在连接智能运维服务';
    let reply = '';
    const throttle = new StreamUiThrottle((t: string) => {
      if (this.cancelStream) {
        return;
      }
      reply = t;
      this.replaceStreaming(t, 'streaming');
      this.scrollToEnd();
      if (this.activity.indexOf('连接') >= 0 || this.activity.indexOf('调用') < 0) {
        this.activity = '已连接，正在分析问题';
      }
    });
    this.streamThrottle = throttle;
    this.lastRetryText = msg;
    try {
      const req: AiChatRequest = {
        message: msg,
        session_id: this.sessionId,
        history: history,
        stream: true
      };
      if (images.length > 0) {
        req.images = images;
      }
      if (files.length > 0) {
        req.files = files;
      }
      reply = await Api.aiChat(req, (t: string) => {
        throttle.push(t);
      }, (act: string) => {
        if (!this.cancelStream) {
          this.activity = act;
        }
      }, (sid: number) => {
        this.sessionId = sid;
      }, (ui: AiUiAction) => {
        if (!this.cancelStream) {
          this.appendStreamingAction(ui);
        }
      });
      throttle.flush();
      if (this.cancelStream) {
        this.replaceStreaming(reply || '已停止生成', 'text');
        return;
      }
      this.replaceStreaming(reply || '', 'text');
      this.activity = '';
      this.scrollToEnd();
    } catch (e) {
      try {
        this.activity = '兼容通道重试中…';
        const legacyReq: AiChatRequest = {
          message: msg,
          session_id: 0,
          history: history,
          stream: true
        };
        if (images.length > 0) {
          legacyReq.images = images;
        }
        if (files.length > 0) {
          legacyReq.files = files;
        }
        reply = await Api.legacyAiChat(legacyReq, (t: string) => {
          throttle.push(t);
        });
        throttle.flush();
        this.replaceStreaming(reply || '', 'text');
        this.activity = '';
        this.scrollToEnd();
      } catch (e2) {
        this.replaceStreaming(ApiClient.formatError(e2 as Object), 'error');
        this.error = ApiClient.formatError(e2 as Object);
        this.activity = '';
      }
    } finally {
      this.streamThrottle = null;
      this.streaming = false;
    }
  }

  async send(): Promise<void> {
    await this.sendText(this.input);
  }

  build() {
    Stack() {
      Column() {
        PageHeader({
          title: 'AI 助手',
          subtitle: this.streaming ? (this.activity || '生成中…') :
            (this.listening ? '正在聆听…' : (this.speaking ? '朗读中' : '')),
          trailing2: this.speaking ? '停止朗读' : '',
          onTrailing2: () => this.voice.stop(),
          trailing: '清空',
          onTrailing: () => this.clearChat()
        })
        ErrorBanner({
          message: this.error,
          showRetry: this.error.length > 0 && this.lastRetryText.length > 0,
          onRetry: () => {
            this.error = '';
            void this.sendText(this.lastRetryText);
          }
        })

        Scroll() {
          Row({ space: 8 }) {
            ForEach(this.caps, (s: string) => {
              Text(s)
                .fontSize(AppTheme.fontMicro)
                .fontColor(this.tMuted)
                .padding({ left: 10, right: 10, top: 5, bottom: 5 })
                .backgroundColor(this.tElevated)
                .borderRadius(AppTheme.rSm)
                .border({ width: 1, color: this.tBorderSoft })
            }, (s: string) => s)
          }
        }
        .scrollable(ScrollDirection.Horizontal)
        .height(34)
        .padding({ left: 12, right: 12, bottom: 6 })

        if (this.messages.length === 0) {
          Column({ space: 14 }) {
            Text('智能运维助手')
              .fontSize(AppTheme.fontTitle)
              .fontWeight(FontWeight.Bold)
              .fontColor(this.tText)
            Text('可查询指标、日志、告警与处置建议。点下方建议快速开始，或直接输入问题。')
              .fontSize(AppTheme.fontSmall)
              .fontColor(this.tMuted)
              .lineHeight(20)
              .width('100%')
            if (this.suggestions.length > 0) {
              ForEach(this.suggestions, (s: string) => {
                Text(s)
                  .fontSize(AppTheme.fontSmall)
                  .fontColor(this.streaming ? this.tMuted : this.tAccent)
                  .opacity(this.streaming ? 0.45 : 1)
                  .width('100%')
                  .padding({ left: 14, right: 14, top: 12, bottom: 12 })
                  .backgroundColor(this.tAccentSoft)
                  .borderRadius(AppTheme.rSm)
                  .enabled(!this.streaming)
                  .onClick(() => {
                    if (!this.streaming) {
                      void this.sendText(s);
                    }
                  })
              }, (s: string) => `sug-${s}`)
            }
          }
          .width('100%')
          .layoutWeight(1)
          .padding({ left: 20, right: 20, top: 24 })
          .alignItems(HorizontalAlign.Start)
        } else {
          if (this.suggestions.length > 0) {
            Scroll() {
              Row({ space: 8 }) {
                ForEach(this.suggestions, (s: string) => {
                  Text(s)
                    .fontSize(AppTheme.fontMicro)
                    .fontColor(this.streaming ? this.tMuted : this.tAccent)
                    .opacity(this.streaming ? 0.4 : 1)
                    .padding({ left: 10, right: 10, top: 8, bottom: 8 })
                    .constraintSize({ minHeight: 36 })
                    .backgroundColor(this.tAccentSoft)
                    .borderRadius(AppTheme.rSm)
                    .maxLines(1)
                    .enabled(!this.streaming)
                    .onClick(() => {
                      if (!this.streaming) {
                        void this.sendText(s);
                      }
                    })
                }, (s: string) => s)
              }
            }
            .scrollable(ScrollDirection.Horizontal)
            .height(40)
            .padding({ left: 12, right: 12, bottom: 8 })
          }

          List({ scroller: this.listScroller }) {
            ForEach(this.messages, (m: ChatMessage, idx: number) => {
              ListItem() {
                Row() {
                  if (m.role === 'user') {
                    Blank().layoutWeight(1)
                  }
                  Column() {
                    Text(m.role === 'user' ? '我' : (m.type === 'diagnosis' ? 'AIOps 诊断' : 'AI'))
                      .fontSize(AppTheme.fontMicro)
                      .fontColor(m.type === 'error' ? this.tDanger : this.tMuted)
                    if (m.role === 'assistant' && m.type === 'streaming' && !(m.content || '').trim()) {
                      Row({ space: 8 }) {
                        LoadingProgress().width(14).height(14).color(this.tAccent)
                        Text(this.activity || '正在生成回复…')
                          .fontSize(AppTheme.fontCaption)
                          .fontColor(this.tMuted)
                      }
                      .margin({ top: 6 })
                    } else if (m.role === 'assistant') {
                      MarkdownText({
                        content: m.content || '',
                        color: m.type === 'error' ? this.tDanger : this.tText,
                        fontSize: AppTheme.fontBody
                      })
                        .margin({ top: 4 })
                        .width('100%')
                    } else {
                      Text(m.content || '')
                        .fontSize(AppTheme.fontBody)
                        .fontColor(this.tText)
                        .margin({ top: 4 })
                    }
                    if (m.role === 'assistant' && m.type === 'error') {
                      Text('重试')
                        .fontSize(AppTheme.fontCaption)
                        .fontColor(this.tAccent)
                        .margin({ top: 6 })
                        .onClick(() => {
                          if (this.lastRetryText) {
                            void this.sendText(this.lastRetryText);
                          }
                        })
                    } else if (m.role === 'assistant' && m.type !== 'streaming' && (m.content || '').trim()) {
                      Text('朗读')
                        .fontSize(AppTheme.fontCaption)
                        .fontColor(this.tAccent)
                        .margin({ top: 6 })
                        .onClick(() => this.voice.speak(m.content))
                    }
                    if (m.role === 'assistant' && m.actions && m.actions.length > 0) {
                      Flex({ wrap: FlexWrap.Wrap }) {
                        ForEach(m.actions, (act: AiUiAction, i: number) => {
                          Text(act.label || act.type || '操作')
                            .fontSize(AppTheme.fontCaption)
                            .fontColor(this.tAccent)
                            .padding({ left: 10, right: 10, top: 6, bottom: 6 })
                            .margin({ top: 6, right: 6 })
                            .backgroundColor(this.tAccentSoft)
                            .borderRadius(AppTheme.rSm)
                            .border({ width: 1, color: this.tAccent })
                            .onClick(() => this.handleUiAction(act))
                        }, (act: AiUiAction, i: number) => `${act.type}_${act.id || ''}_${i}`)
                      }
                      .width('100%')
                    }
                  }
                  .alignItems(HorizontalAlign.Start)
                  .padding(AppTheme.spaceMd)
                  .backgroundColor(m.role === 'user' ? this.tAccentSoft : this.tCard)
                  .borderRadius(AppTheme.rSm)
                  .border({
                    width: 1,
                    color: m.type === 'error' ? this.tDanger : (m.role === 'user' ? this.tAccentSoft : this.tBorderSoft)
                  })
                  .constraintSize({ maxWidth: '82%' })
                  if (m.role !== 'user') {
                    Blank().layoutWeight(1)
                  }
                }
                .width('100%')
              }
              .margin({ left: 12, right: 12, bottom: 8 })
            }, (m: ChatMessage, idx: number) => `${idx}-${m.role}-${m.type || 't'}-${(m.content || '').length}`)
          }
          .layoutWeight(1)
        }

        if (this.pending.length > 0) {
          Scroll() {
            Row({ space: 6 }) {
              ForEach(this.pending, (p: PendingAiAttachment, i: number) => {
                Text(`${p.name} ×`)
                  .fontSize(AppTheme.fontMicro)
                  .fontColor(this.tAccent)
                  .padding({ left: 10, right: 10, top: 6, bottom: 6 })
                  .constraintSize({ minHeight: 32 })
                  .backgroundColor(this.tAccentSoft)
                  .borderRadius(AppTheme.rXs)
                  .onClick(() => this.removePending(i))
              }, (p: PendingAiAttachment, i: number) => `att-${i}-${p.kind}-${p.name}`)
            }
          }
          .scrollable(ScrollDirection.Horizontal)
          .height(36)
          .padding({ left: 12, right: 12, bottom: 4 })
        }

        Row({ space: 8 }) {
          Row({ space: 2 }) {
            Column() {
              AppSymbol({
                kind: 'attach',
                tint: this.attaching ? this.tAccent : this.tMuted,
                iconSize: 18
              })
            }
            .width(36)
            .height(36)
            .justifyContent(FlexAlign.Center)
            .onClick(() => {
              if (!this.streaming) {
                this.showAttachSheet();
              }
            })

            Column() {
              AppSymbol({
                kind: 'mic',
                tint: this.listening ? this.tAccent : this.tMuted,
                iconSize: 18
              })
            }
            .width(36)
            .height(36)
            .justifyContent(FlexAlign.Center)
            .onClick(() => {
              if (!this.streaming) {
                this.onMic();
              }
            })

            TextArea({
              text: this.input,
              placeholder: this.listening ?
                (this.sttPartial.length > 0 ? `聆听中: ${this.sttPartial}` : '正在聆听…') :
                '输入问题…'
            })
              .onChange((v: string) => {
                this.input = v;
                if (!this.listening) {
                  this.sttBaseInput = v;
                }
              })
              .layoutWeight(1)
              .constraintSize({ minHeight: 36, maxHeight: 100 })
              .backgroundColor(Color.Transparent)
              .fontColor(this.tText)
              .placeholderColor(this.tMuted)
              .padding({ left: 4, right: 8, top: 8, bottom: 8 })
              .enabled(!this.streaming)
          }
          .layoutWeight(1)
          .height(44)
          .padding({ left: 4, right: 8 })
          .backgroundColor(this.tElevated)
          .borderRadius(AppTheme.rLg)
          .border({
            width: AppTheme.hairline,
            color: this.listening ? this.tAccent : this.tBorderSoft
          })
          .alignItems(VerticalAlign.Center)

          Button() {
            AppSymbol({
              kind: this.streaming ? 'stop' : 'send',
              tint: this.tOnAccent,
              iconSize: 14
            })
          }
          .width(AppTheme.sendBtn)
          .height(AppTheme.sendBtn)
          .backgroundColor(this.streaming ? this.tWarning : this.tAccent)
          .borderRadius(AppTheme.sendBtn / 2)
          .enabled(this.streaming || this.input.trim().length > 0 || this.pending.length > 0)
          .onClick(() => {
            if (this.streaming) {
              this.stopGen();
            } else {
              void this.send();
            }
          })
        }
        .width('100%')
        .padding({ left: 12, right: 12, top: 8, bottom: 10 })
        .alignItems(VerticalAlign.Center)
      }
      .width('100%')
      .height('100%')
      .backgroundColor(this.tBg)

      ToastHost({ message: this.toastMsg, isError: this.toastError })
    }
    .width('100%')
    .height('100%')
  }
}

@Entry
@Component
struct AiCopilotPage {
  @StorageLink('aiops_theme_rev') themeRev: number = 0;

  @StorageLink('aiops_bg') tBg: string = '#080C14';
  @StorageLink('aiops_card') tCard: string = '#111722';
  @StorageLink('aiops_elevated') tElevated: string = '#192130';
  @StorageLink('aiops_text') tText: string = '#E6E8F0';
  @StorageLink('aiops_muted') tMuted: string = '#9AA3B8';
  @StorageLink('aiops_accent') tAccent: string = '#4F7FFF';
  @StorageLink('aiops_accent_soft') tAccentSoft: string = '#1E3166';
  @StorageLink('aiops_accent_on') tAccentOn: string = '#D6E2FF';
  @StorageLink('aiops_success') tSuccess: string = '#00A86B';
  @StorageLink('aiops_success_soft') tSuccessSoft: string = '#0F3A2C';
  @StorageLink('aiops_warning') tWarning: string = '#F59E0B';
  @StorageLink('aiops_warning_soft') tWarningSoft: string = '#3A2A10';
  @StorageLink('aiops_danger') tDanger: string = '#EF4444';
  @StorageLink('aiops_danger_soft') tDangerSoft: string = '#3A1614';
  @StorageLink('aiops_border') tBorder: string = '#39445A';
  @StorageLink('aiops_border_soft') tBorderSoft: string = '#222C3D';
  @StorageLink('aiops_on_accent') tOnAccent: string = '#FFFFFF';
  @StorageLink('aiops_nav_bar') tNavBar: string = '#111722';
  @StorageLink('aiops_scrim') tScrim: string = '#99000000';
  @StorageLink('aiops_shadow') tShadow: string = '#40000000';

  build() {
    Column() {
      AiCopilotTab()
    }
    .width('100%')
    .height('100%')
  }
}
