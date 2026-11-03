let currentSessionId = null;
let eventSource = null;
let isRunning = false;

const statusEl = document.getElementById('status');
const chatEl = document.getElementById('chat');
const messageEl = document.getElementById('message');
const sendBtn = document.getElementById('send');
const stopBtn = document.getElementById('stop');
const createSessionBtn = document.getElementById('createSession');
const modelSelect = document.getElementById('model');
const workspaceInput = document.getElementById('workspace');

document.getElementById('createSession').addEventListener('click', createSession);
document.getElementById('send').addEventListener('click', sendMessage);
document.getElementById('stop').addEventListener('click', stopSession);
document.getElementById('message').addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        sendMessage();
    }
});

async function createSession() {
    const model = modelSelect.value;
    const workspace = workspaceInput.value;

    try {
        const response = await fetch('/session', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ model, max_steps: 10, workspace })
        });

        if (!response.ok) {
            throw new Error('Failed to create session');
        }

        const session = await response.json();
        currentSessionId = session.id;
        chatEl.innerHTML = '';
        appendMessage('system', `Session created: ${session.id}`);
        connectStream();
        updateStatus('Session ready');
    } catch (error) {
        appendMessage('error', error.message);
        updateStatus('Error');
    }
}

async function sendMessage() {
    if (!currentSessionId) {
        appendMessage('error', 'Please create a session first by clicking "New Session"');
        return;
    }
    if (isRunning) return;

    const content = messageEl.value.trim();
    if (!content) return;

    appendMessage('user', content);
    messageEl.value = '';
    isRunning = true;
    sendBtn.disabled = true;
    stopBtn.disabled = false;
    updateStatus('Running...');

    try {
        const response = await fetch(`/session/${currentSessionId}/message`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ content })
        });

        if (!response.ok) {
            throw new Error('Failed to send message');
        }
    } catch (error) {
        appendMessage('error', error.message);
        isRunning = false;
        sendBtn.disabled = false;
        stopBtn.disabled = true;
        updateStatus('Error');
    }
}

async function stopSession() {
    if (!currentSessionId) return;

    try {
        await fetch(`/session/${currentSessionId}/stop`, { method: 'POST' });
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
        appendMessage('reasoning', data.content);
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
        isRunning = false;
        sendBtn.disabled = false;
        stopBtn.disabled = true;
        updateStatus('Done: ' + data.finish_reason);
    });

    eventSource.onerror = () => {
        updateStatus('Stream error');
    };
}

function appendMessage(role, content, append = false) {
    let messageEl = document.getElementById(`msg-${role}`);

    if (append && messageEl) {
        messageEl.textContent += content;
    } else {
        messageEl = document.createElement('div');
        messageEl.id = `msg-${role}`;
        messageEl.className = `message ${role}`;
        messageEl.textContent = content;
        chatEl.appendChild(messageEl);
    }

    chatEl.scrollTop = chatEl.scrollHeight;
}

function updateStatus(text) {
    statusEl.textContent = text;
}
