let currentSessionId = null;
let eventSource = null;
let isRunning = false;

const statusEl = document.getElementById('status');
const chatEl = document.getElementById('chat');
const messageEl = document.getElementById('message');
const actionBtn = document.getElementById('actionBtn');
const createSessionBtn = document.getElementById('createSession');
const modelSelect = document.getElementById('model');
const workspaceInput = document.getElementById('workspace');
const securityModeSelect = document.getElementById('securityMode');
const historyList = document.getElementById('history-list');

document.getElementById('createSession').addEventListener('click', createSession);
actionBtn.addEventListener('click', handleActionClick);
document.getElementById('message').addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        if (!isRunning) {
            sendMessage();
        }
    }
});

function handleActionClick() {
    if (isRunning) {
        stopSession();
    } else {
        sendMessage();
    }
}

function setRunningState(running) {
    isRunning = running;
    if (running) {
        actionBtn.textContent = 'Stop';
        actionBtn.className = 'stop';
        actionBtn.disabled = false;
    } else {
        actionBtn.textContent = 'Send';
        actionBtn.className = '';
        actionBtn.disabled = false;
    }
}

// Загрузка настроек и истории промптов при старте
initApp();

async function initApp() {
    await loadAvailableModels();
    await restoreSession();
    await loadSettings();
    await loadPromptHistory();
}

async function loadAvailableModels() {
    try {
        const res = await fetch('/models');
        if (res.ok) {
            const data = await res.json();
            if (data.models && Array.isArray(data.models) && data.models.length > 0) {
                const currentVal = modelSelect.value;
                modelSelect.innerHTML = '';
                data.models.forEach((name) => {
                    const opt = document.createElement('option');
                    opt.value = name;
                    opt.textContent = name;
                    modelSelect.appendChild(opt);
                });
                if (currentVal && data.models.includes(currentVal)) {
                    modelSelect.value = currentVal;
                }
            }
        }
    } catch (e) {
        console.error('Failed to load available models:', e);
    }
}

async function restoreSession() {
    const savedSessionId = localStorage.getItem('multis_session_id');
    if (!savedSessionId) {
        return;
    }

    try {
        const response = await fetch(`/session/${savedSessionId}`);
        if (response.ok) {
            const session = await response.json();
            currentSessionId = session.id;
            chatEl.innerHTML = '';
            appendMessage('system', `Session restored: ${session.id} (security: ${session.security_mode || 'default'})`);
            if (session.messages && Array.isArray(session.messages)) {
                session.messages.forEach((msg) => {
                    appendMessage(msg.role, msg.content);
                });
            }
            if (session.model) {
                modelSelect.value = session.model;
            }
            if (session.workspace) {
                workspaceInput.value = session.workspace;
            }
            if (session.security_mode && securityModeSelect) {
                securityModeSelect.value = session.security_mode;
            }
            connectStream();
            updateStatus('Session ready');
        } else {
            localStorage.removeItem('multis_session_id');
        }
    } catch (e) {
        console.error('Failed to restore session:', e);
    }
}

async function loadSettings() {
    try {
        const res = await fetch('/settings');
        if (res.ok) {
            const data = await res.json();
            if (data.last_model) {
                modelSelect.value = data.last_model;
            }
            if (data.last_workspace) {
                workspaceInput.value = data.last_workspace;
            }
            if (data.security_mode && securityModeSelect) {
                securityModeSelect.value = data.security_mode;
            }
        }
    } catch (e) {
        console.error('Failed to load settings:', e);
    }
}

async function saveSettings(lastModel, securityMode, lastWorkspace) {
    try {
        const payload = {};
        if (lastModel !== undefined) payload.last_model = lastModel;
        if (securityMode !== undefined) payload.security_mode = securityMode;
        if (lastWorkspace !== undefined) payload.last_workspace = lastWorkspace;

        await fetch('/settings', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        });
    } catch (e) {
        console.error('Failed to save settings:', e);
    }
}

async function loadPromptHistory() {
    try {
        const res = await fetch('/prompts/history?limit=30');
        if (res.ok) {
            const items = await res.json();
            if (historyList) {
                historyList.innerHTML = '';
                const seen = new Set();
                items.forEach((item) => {
                    if (!seen.has(item.prompt)) {
                        seen.add(item.prompt);
                        const opt = document.createElement('option');
                        opt.value = item.prompt;
                        historyList.appendChild(opt);
                    }
                });
            }
        }
    } catch (e) {
        console.error('Failed to load prompt history:', e);
    }
}

async function createSession() {
    const model = modelSelect.value;
    const workspace = workspaceInput.value;
    const securityMode = securityModeSelect ? securityModeSelect.value : 'default';

    // Сохраняем выбранную модель, директорию и режим
    saveSettings(model, securityMode, workspace);

    try {
        const response = await fetch('/session', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                model,
                max_steps: 10,
                workspace,
                security_mode: securityMode
            })
        });

        if (!response.ok) {
            throw new Error('Failed to create session');
        }

        const session = await response.json();
        currentSessionId = session.id;
        localStorage.setItem('multis_session_id', session.id);
        chatEl.innerHTML = '';
        appendMessage('system', `Session created: ${session.id} (security: ${session.security_mode || securityMode})`);
        connectStream();
        updateStatus('Session ready');
    } catch (error) {
        appendMessage('error', error.message);
        updateStatus('Error');
    }
}

async function sendMessage() {
    if (isRunning) return;

    const content = messageEl.value.trim();
    if (!content) return;

    if (!currentSessionId) {
        await createSession();
        if (!currentSessionId) {
            return;
        }
    }

    appendMessage('user', content);
    messageEl.value = '';
    setRunningState(true);
    updateStatus('Running...');

    try {
        const response = await fetch(`/session/${currentSessionId}/message`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ content, model: modelSelect.value, workspace: workspaceInput.value })
        });

        if (!response.ok) {
            throw new Error('Failed to send message');
        }

        // Обновляем список истории
        loadPromptHistory();
    } catch (error) {
        appendMessage('error', error.message);
        setRunningState(false);
        updateStatus('Error');
    }
}

async function stopSession() {
    if (!currentSessionId) return;

    try {
        await fetch(`/session/${currentSessionId}/stop`, { method: 'POST' });
        setRunningState(false);
        updateStatus('Stopped');
    } catch (error) {
        appendMessage('error', error.message);
    }
}

function connectStream() {
    if (!currentSessionId) return;

    if (eventSource) {
        eventSource.close();
    }

    eventSource = new EventSource(`/session/${currentSessionId}/stream`);

    eventSource.addEventListener('reasoning', (e) => {
        const data = JSON.parse(e.data);
        appendMessage('reasoning', data.content, true);
    });

    eventSource.addEventListener('message', (e) => {
        const data = JSON.parse(e.data);
        appendMessage('assistant', data.content, true);
    });

    eventSource.addEventListener('tool_call', (e) => {
        const data = JSON.parse(e.data);
        appendToolCall(data.name, data.arguments);
    });

    eventSource.addEventListener('tool_result', (e) => {
        const data = JSON.parse(e.data);
        appendToolResult(data.output, data.status);
    });

    eventSource.addEventListener('error', (e) => {
        const data = JSON.parse(e.data);
        appendMessage('error', data.message);
    });

    eventSource.addEventListener('done', (e) => {
        const data = JSON.parse(e.data);
        setRunningState(false);
        currentAssistantEl = null;
        currentReasoningEl = null;
        updateStatus('Done: ' + data.finish_reason);
    });

    eventSource.onerror = () => {
        updateStatus('Stream error');
    };
}

let currentAssistantEl = null;
let currentReasoningEl = null;
let pendingToolElements = [];

const SCROLL_THRESHOLD = 50;

function isNearBottom() {
    return chatEl.scrollHeight - chatEl.scrollTop - chatEl.clientHeight < SCROLL_THRESHOLD;
}

function scrollToBottom() {
    chatEl.scrollTop = chatEl.scrollHeight;
}

function formatToolArgs(args) {
    if (!args || Object.keys(args).length === 0) return '';
    return Object.entries(args)
        .map(([key, val]) => {
            const v = typeof val === 'string' ? val : JSON.stringify(val);
            return `${key}: ${v}`;
        })
        .join(', ');
}

function appendToolCall(name, args) {
    currentAssistantEl = null;
    currentReasoningEl = null;
    const stickToBottom = isNearBottom();

    const details = document.createElement('details');
    details.className = 'message tool-block';

    const summary = document.createElement('summary');
    const nameSpan = document.createElement('span');
    nameSpan.className = 'tool-name';
    nameSpan.textContent = name;
    summary.appendChild(nameSpan);

    const argsStr = formatToolArgs(args);
    if (argsStr) {
        const argsSpan = document.createElement('span');
        argsSpan.className = 'tool-args';
        argsSpan.textContent = argsStr;
        summary.appendChild(argsSpan);
    }

    details.appendChild(summary);

    const resultDiv = document.createElement('div');
    resultDiv.className = 'tool-result-content';
    resultDiv.textContent = '⏳ Running...';
    details.appendChild(resultDiv);

    chatEl.appendChild(details);
    pendingToolElements.push(details);
    if (stickToBottom) scrollToBottom();
}

function appendToolResult(output, status) {
    const stickToBottom = isNearBottom();
    const details = pendingToolElements.shift();
    if (details) {
        const resultDiv = details.querySelector('.tool-result-content');
        if (resultDiv) {
            resultDiv.textContent = output || '(empty)';
            if (status === 'error') {
                resultDiv.classList.add('tool-error');
            }
        }
    } else {
        appendMessage('tool_result', output);
        return;
    }
    if (stickToBottom) scrollToBottom();
}

function appendMessage(role, content, append = false) {
    const stickToBottom = role === 'user' || isNearBottom();
    if (role === 'assistant') {
        currentReasoningEl = null;
        if (!currentAssistantEl || !append) {
            currentAssistantEl = document.createElement('div');
            currentAssistantEl.className = 'message assistant';
            chatEl.appendChild(currentAssistantEl);
        }
        currentAssistantEl.textContent += content;
    } else if (role === 'reasoning') {
        currentAssistantEl = null;
        if (!currentReasoningEl || !append) {
            currentReasoningEl = document.createElement('div');
            currentReasoningEl.className = 'message reasoning';
            chatEl.appendChild(currentReasoningEl);
        }
        currentReasoningEl.textContent += content;
    } else {
        currentAssistantEl = null;
        currentReasoningEl = null;
        const messageEl = document.createElement('div');
        messageEl.className = `message ${role}`;
        messageEl.textContent = content;
        chatEl.appendChild(messageEl);
    }

    if (stickToBottom) scrollToBottom();
}

function updateStatus(text) {
    statusEl.textContent = text;
}

// Цитирование выделенного текста в ленте
const quoteBtn = document.createElement('button');
quoteBtn.className = 'quote-btn hidden';
quoteBtn.textContent = '❝ Quote';
document.body.appendChild(quoteBtn);

function hideQuoteButton() {
    quoteBtn.classList.add('hidden');
}

function updateQuoteButton() {
    const sel = window.getSelection();
    if (!sel || sel.isCollapsed || !sel.toString().trim()) {
        hideQuoteButton();
        return;
    }
    const range = sel.getRangeAt(0);
    if (!chatEl.contains(range.commonAncestorContainer)) {
        hideQuoteButton();
        return;
    }
    const rect = range.getBoundingClientRect();
    quoteBtn.style.top = (rect.bottom + 4) + 'px';
    quoteBtn.style.left = rect.left + 'px';
    quoteBtn.classList.remove('hidden');
}

chatEl.addEventListener('mouseup', updateQuoteButton);
chatEl.addEventListener('keyup', updateQuoteButton);
chatEl.addEventListener('scroll', hideQuoteButton);
document.addEventListener('mousedown', (e) => {
    if (!quoteBtn.contains(e.target)) hideQuoteButton();
});
quoteBtn.addEventListener('mousedown', (e) => e.preventDefault());
quoteBtn.addEventListener('click', () => {
    const sel = window.getSelection();
    if (sel && !sel.isCollapsed) {
        const text = sel.toString().replace(/\s+/g, ' ').trim();
        const start = messageEl.selectionStart ?? messageEl.value.length;
        const end = messageEl.selectionEnd ?? messageEl.value.length;
        const quote = '[quote] ' + text + ' [/quote] ';
        messageEl.value = messageEl.value.slice(0, start) + quote + messageEl.value.slice(end);
        messageEl.selectionStart = messageEl.selectionEnd = start + quote.length;
        messageEl.focus();
        hideQuoteButton();
        sel.removeAllRanges();
    }
});

// Directory picker для выбора workspace
const browseBtn = document.getElementById('browseDir');
const dirPicker = document.getElementById('dirPicker');
let browsePath = '.';

browseBtn.addEventListener('click', () => {
    if (dirPicker.classList.contains('hidden')) {
        browsePath = workspaceInput.value || '.';
        loadDirs(browsePath);
        dirPicker.classList.remove('hidden');
    } else {
        dirPicker.classList.add('hidden');
    }
});

document.addEventListener('click', (e) => {
    if (!dirPicker.classList.contains('hidden') && !dirPicker.contains(e.target) && e.target !== browseBtn) {
        dirPicker.classList.add('hidden');
    }
});

async function loadDirs(path) {
    browsePath = path;
    try {
        const res = await fetch('/fs/dirs?path=' + encodeURIComponent(path));
        if (!res.ok) {
            dirPicker.innerHTML = '<div class="dir-path">' + (await res.text()) + '</div>';
            return;
        }
        const data = await res.json();
        browsePath = data.path;

        dirPicker.innerHTML = '';
        const pathEl = document.createElement('div');
        pathEl.className = 'dir-path';
        pathEl.textContent = data.path;
        dirPicker.appendChild(pathEl);

        if (data.parent && data.parent !== data.path) {
            const parentEl = document.createElement('div');
            parentEl.className = 'dir-item';
            parentEl.textContent = '⬅ ..';
            parentEl.addEventListener('click', () => loadDirs(data.parent));
            dirPicker.appendChild(parentEl);
        }

        (data.dirs || []).forEach((d) => {
            const el = document.createElement('div');
            el.className = 'dir-item';
            el.textContent = '📁 ' + d.name;
            el.title = d.path;
            el.addEventListener('click', () => loadDirs(d.path));
            dirPicker.appendChild(el);
        });

        const selectBtn = document.createElement('button');
        selectBtn.className = 'dir-select';
        selectBtn.textContent = 'Use this directory';
        selectBtn.addEventListener('click', () => {
            workspaceInput.value = browsePath;
            dirPicker.classList.add('hidden');
            saveSettings(undefined, undefined, browsePath);
        });
        dirPicker.appendChild(selectBtn);
    } catch (e) {
        dirPicker.innerHTML = '<div class="dir-path">Failed to load: ' + e.message + '</div>';
    }
}
