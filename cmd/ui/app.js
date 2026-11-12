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
    await loadSettings();
    await loadPromptHistory();
    await restoreSession();
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
            if (data.security_mode && securityModeSelect) {
                securityModeSelect.value = data.security_mode;
            }
        }
    } catch (e) {
        console.error('Failed to load settings:', e);
    }
}

async function saveSettings(lastModel, securityMode) {
    try {
        const payload = {};
        if (lastModel !== undefined) payload.last_model = lastModel;
        if (securityMode !== undefined) payload.security_mode = securityMode;

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

    // Сохраняем выбранную модель и режим
    saveSettings(model, securityMode);

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
            body: JSON.stringify({ content, model: modelSelect.value })
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
        appendMessage('tool_call', `${data.name}(${JSON.stringify(data.arguments)})`);
    });

    eventSource.addEventListener('tool_result', (e) => {
        const data = JSON.parse(e.data);
        appendMessage('tool_result', data.output);
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

function appendMessage(role, content, append = false) {
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

    chatEl.scrollTop = chatEl.scrollHeight;
}

function updateStatus(text) {
    statusEl.textContent = text;
}
