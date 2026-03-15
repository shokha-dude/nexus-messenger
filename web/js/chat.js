
// ========== СОСТОЯНИЕ ПРИЛОЖЕНИЯ ==========
let ws;
let currentUser = null;
let selectedUser = null;
let selectedGroup = null;
let currentChatType = 'private';
let typingTimer;
let isTyping = false;
const TYPING_TIMEOUT = 2000;
let allUsers = [];
let allGroups = [];

// ========== УПРАВЛЕНИЕ ВКЛАДКАМИ ==========
function showTab(tab) {
    document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
    document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));

    document.getElementById(`tab-${tab}`).classList.add('active');
    document.getElementById(`${tab}-tab`).classList.add('active');
}

// ========== УПРАВЛЕНИЕ СТАТУСОМ ==========
function showStatus(message) {
    const statusEl = document.getElementById('status');
    statusEl.innerText = message;
    setTimeout(() => statusEl.innerText = '', 3000);
}

// ========== ПОДКЛЮЧЕНИЕ ==========
function connect() {
    const token = document.getElementById('token').value;
    if (!token) {
        alert('Please enter token');
        return;
    }

    try {
        const payload = JSON.parse(atob(token.split('.')[1]));
        currentUser = payload.username;
        console.log("Connected as:", currentUser);
    } catch (e) {
        console.log('Could not decode token');
    }

    ws = new WebSocket(`ws://${window.location.host}/ws?token=${token}`);

    ws.onopen = () => {
        document.getElementById('connection-status').innerText = 'Connected';
        document.getElementById('connection-status').style.background = '#4caf50';
        document.getElementById('container').style.display = 'flex';
        getOnlineUsers();
        getMyGroups();
    };

    ws.onclose = () => {
        document.getElementById('connection-status').innerText = 'Disconnected';
        document.getElementById('connection-status').style.background = '#f44336';
        document.getElementById('container').style.display = 'none';
    };

    ws.onmessage = (event) => {
        const data = JSON.parse(event.data);
        handleMessage(data);
    };

    ws.onerror = (error) => {
        console.error('WebSocket error:', error);
        showStatus('Connection error');
    };
}

// ========== ОБРАБОТКА СООБЩЕНИЙ ==========
function handleMessage(data) {
    switch (data.type) {
        case 'online_users':
            updateUsersList(data.payload);
            break;
        case 'typing_status':
            displayTypingStatus(data.payload);
            break;
        case 'private_message':
            if (currentChatType === 'private' && selectedUser === data.payload.from) {
                displayPrivateMessage(data.payload);
            } else {
                showStatus(`💬 New message from ${data.payload.from}`);
            }
            break;
        case 'group_message':
            if (currentChatType === 'group' && selectedGroup?.id === data.payload.conversation_id) {
                displayGroupMessage(data.payload);
            } else {
                showStatus(`👥 New message in ${data.payload.group_name} from ${data.payload.from}`);
            }
            break;
        case 'my_groups':
            updateGroupsList(data.payload);
            break;
        case 'group_created':
            showStatus(`Group "${data.payload.name}" created!`);
            getMyGroups();
            break;
        case 'group_info':
            displayGroupInfo(data.payload);
            break;
        case 'added_to_group':
            showStatus(`✨ Added to group: ${data.payload.group_name}`);
            getMyGroups();
            break;
        case 'group_history':
            displayGroupHistory(data.payload);
            break;
        case 'private_history':
            displayPrivateHistory(data.payload);
            break;
        case 'message_sent':
            document.querySelectorAll('.sending-indicator').forEach(el => el.remove());
            break;
        case 'error':
            alert('Error: ' + data.payload.message);
            break;
    }
}

// ========== ОБНОВЛЕНИЕ СПИСКОВ ==========
function updateUsersList(users) {
    allUsers = users;
    const list = document.getElementById('users-list');
    if (!list) return;

    document.getElementById('online-count').textContent = users.length - 1;
    renderUsersList(users);
    updateAvailableUsers();
}

function renderUsersList(users, filter = '') {
    const list = document.getElementById('users-list');
    if (!list) return;

    list.innerHTML = '';

    const filteredUsers = users.filter(u =>
        u.username !== currentUser &&
        u.username.toLowerCase().includes(filter.toLowerCase())
    );

    if (filteredUsers.length === 0) {
        list.innerHTML = '<li style="padding:15px; text-align:center; color:#999;">No users found</li>';
        return;
    }

    filteredUsers.forEach(user => {
        const li = document.createElement('li');
        const avatar = user.username.charAt(0).toUpperCase();
        const color = getColorForUser(user.username);

        li.innerHTML = `
            <div class="avatar" style="background: ${color}">${avatar}</div>
            <div class="item-info">
                <div class="item-name">${user.username}</div>
                <div class="item-status">
                    <span class="online-dot"></span> online
                </div>
            </div>
        `;
        li.onclick = () => selectUser(user.username);
        if (selectedUser === user.username) li.classList.add('selected');
        list.appendChild(li);
    });
}

function getColorForUser(username) {
    const colors = ['#2196f3', '#f44336', '#4caf50', '#ff9800', '#9c27b0', '#00bcd4'];
    let hash = 0;
    for (let i = 0; i < username.length; i++) {
        hash = username.charCodeAt(i) + ((hash << 5) - hash);
    }
    return colors[Math.abs(hash) % colors.length];
}

function filterUsers() {
    const filter = document.getElementById('user-search').value;
    renderUsersList(allUsers, filter);
}

function updateGroupsList(groups) {
    allGroups = groups || [];
    document.getElementById('groups-count').textContent = groups?.length || 0;

    const list = document.getElementById('groups-list');
    if (!list) return;

    renderGroupsList(groups);
}

function renderGroupsList(groups, filter = '') {
    const list = document.getElementById('groups-list');
    if (!list) return;

    list.innerHTML = '';

    if (!groups?.length) {
        list.innerHTML = '<li style="padding:15px; text-align:center; color:#999;">No groups yet</li>';
        return;
    }

    const filteredGroups = groups.filter(g =>
        g.name && g.name.toLowerCase().includes(filter.toLowerCase())
    );

    if (filteredGroups.length === 0) {
        list.innerHTML = '<li style="padding:15px; text-align:center; color:#999;">No groups found</li>';
        return;
    }

    filteredGroups.forEach(group => {
        const li = document.createElement('li');
        const avatar = group.name?.charAt(0).toUpperCase() || 'G';

        li.innerHTML = `
            <div class="avatar group-avatar">${avatar}</div>
            <div class="item-info">
                <div class="item-name">${group.name || 'Unnamed Group'}</div>
                <div class="item-status">Group</div>
            </div>
        `;
        li.onclick = () => selectGroup(group);
        if (selectedGroup?.id === group.id) li.classList.add('selected');
        list.appendChild(li);
    });
}

function filterGroups() {
    const filter = document.getElementById('group-search').value;
    renderGroupsList(allGroups, filter);
}

// ========== ВЫБОР ПОЛЬЗОВАТЕЛЯ/ГРУППЫ ==========
function selectUser(username) {
    selectedUser = username;
    selectedGroup = null;
    currentChatType = 'private';
    document.getElementById('current-recipient').textContent = username;
    document.getElementById('chat-type').textContent = '👤';
    document.getElementById('typing-indicator').style.display = 'none';
    document.getElementById('group-info-container').style.display = 'none';

    document.querySelectorAll('#users-list li').forEach(li => {
        li.classList.toggle('selected', li.querySelector('.item-name')?.textContent === username);
    });
    document.querySelectorAll('#groups-list li').forEach(li => li.classList.remove('selected'));

    document.getElementById('messages').innerHTML = '';

    if (ws?.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({
            type: 'get_private_history',
            payload: { with_user: username, limit: 50 }
        }));
    }
}

function selectGroup(group) {
    selectedGroup = group;
    selectedUser = null;
    currentChatType = 'group';
    document.getElementById('current-recipient').textContent = group.name;
    document.getElementById('chat-type').textContent = '👥';
    document.getElementById('typing-indicator').style.display = 'none';
    document.getElementById('group-info-container').style.display = 'block';

    document.querySelectorAll('#groups-list li').forEach(li => {
        li.classList.toggle('selected', li.getAttribute('data-group-id') == group.id);
    });
    document.querySelectorAll('#users-list li').forEach(li => li.classList.remove('selected'));

    const messagesDiv = document.getElementById('messages');
    messagesDiv.innerHTML = '<div style="text-align: center; color: #999;">Loading history...</div>';

    if (ws?.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({
            type: 'get_group_history',
            payload: { conversation_id: group.id, limit: 50 }
        }));
        ws.send(JSON.stringify({
            type: 'get_group_info',
            payload: { conversation_id: group.id }
        }));
    }
}

// ========== ОТОБРАЖЕНИЕ СООБЩЕНИЙ ==========
function displayPrivateMessage(payload) {
    const welcomeMsg = document.getElementById('welcome-message');
    if (welcomeMsg) welcomeMsg.remove();

    const messagesDiv = document.getElementById('messages');
    const messageDiv = document.createElement('div');
    messageDiv.className = `message ${payload.from === currentUser ? 'my-message' : ''}`;

    const time = new Date(payload.created_at).toLocaleTimeString();
    messageDiv.innerHTML = `
        <span class="sender">${payload.from}:</span>
        <span class="text">${payload.text}</span>
        <span class="time">${time}</span>
    `;

    messagesDiv.appendChild(messageDiv);
    messagesDiv.scrollTop = messagesDiv.scrollHeight;
}

function displayGroupMessage(payload, checkWelcome = true) {
    if (checkWelcome) {
        const welcomeMsg = document.getElementById('welcome-message');
        if (welcomeMsg) welcomeMsg.remove();
    }

    const messagesDiv = document.getElementById('messages');
    const messageDiv = document.createElement('div');
    messageDiv.className = `message ${payload.from === currentUser ? 'my-message' : ''} group-message`;

    const time = new Date(payload.created_at).toLocaleTimeString();
    messageDiv.innerHTML = `
        <span class="sender">${payload.from}:</span>
        <span class="text">${payload.text}</span>
        <span class="time">${time}</span>
    `;

    messagesDiv.appendChild(messageDiv);
    messagesDiv.scrollTop = messagesDiv.scrollHeight;
}

function displayPrivateHistory(payload) {
    if (selectedUser !== payload.with_user) return;

    const messagesDiv = document.getElementById('messages');
    messagesDiv.innerHTML = '';

    if (payload.messages?.length) {
        payload.messages.forEach(msg => {
            displayPrivateMessage({
                from: msg.sender_username,
                text: msg.text,
                created_at: msg.created_at
            });
        });
    } else {
        messagesDiv.innerHTML = `
            <div id="welcome-message" style="text-align: center; color: #999; padding: 20px;">
                No messages yet. Start conversation with ${payload.with_user}!
            </div>
        `;
    }
}

function displayGroupHistory(payload) {
    const messagesDiv = document.getElementById('messages');
    messagesDiv.innerHTML = '';

    if (payload.messages?.length) {
        payload.messages.forEach(msg => {
            displayGroupMessage({
                from: msg.sender_username,
                text: msg.text,
                created_at: msg.created_at
            }, false);
        });
    } else {
        messagesDiv.innerHTML = `
            <div id="welcome-message" style="text-align: center; color: #999; padding: 20px;">
                No messages yet in group ${payload.group_name}. Start the conversation!
            </div>
        `;
    }
}

function displayGroupInfo(groupInfo) {
    const container = document.getElementById('group-info-container');
    const membersList = groupInfo.members.map(m => `<li>${m}</li>`).join('');

    container.innerHTML = `
        <strong>Group: ${groupInfo.name}</strong><br>
        <span style="color:#666;">Created by: ${groupInfo.created_by}</span>
        <div style="margin-top:10px;">
            <strong>Members:</strong>
            <ul style="margin:5px 0 0 20px;">${membersList}</ul>
        </div>
    `;
}

function displayTypingStatus(payload) {
    const indicator = document.getElementById('typing-indicator');

    if (!payload.in_group && currentChatType === 'private' && selectedUser === payload.from) {
        indicator.textContent = `${payload.from} is typing...`;
        indicator.style.display = payload.is_typing ? 'inline' : 'none';
    } else if (payload.in_group && currentChatType === 'group' && selectedGroup?.id === payload.in_group) {
        indicator.textContent = `${payload.from} is typing...`;
        indicator.style.display = payload.is_typing ? 'inline' : 'none';
    }
}

// ========== ОТПРАВКА СООБЩЕНИЙ ==========
function sendMessage() {
    const input = document.getElementById('message-input');
    const text = input.value.trim();
    if (!text) return;

    if (!ws || ws.readyState !== WebSocket.OPEN) {
        alert('WebSocket not connected');
        return;
    }

    if (currentChatType === 'private') {
        if (!selectedUser) {
            alert('Select a user first');
            return;
        }

        const messagesDiv = document.getElementById('messages');
        const messageDiv = document.createElement('div');
        messageDiv.className = 'message my-message';
        messageDiv.innerHTML = `
            <span class="sender">Me to ${selectedUser}:</span>
            <span class="text">${text}</span>
            <span class="time">${new Date().toLocaleTimeString()}</span>
            <span class="sending-indicator"> (sending...)</span>
        `;
        messagesDiv.appendChild(messageDiv);
        messagesDiv.scrollTop = messagesDiv.scrollHeight;

        ws.send(JSON.stringify({
            type: 'private_message',
            payload: { to: selectedUser, text }
        }));

    } else if (currentChatType === 'group') {
        if (!selectedGroup) {
            alert('Select a group first');
            return;
        }

        const messagesDiv = document.getElementById('messages');
        const messageDiv = document.createElement('div');
        messageDiv.className = 'message my-message';
        messageDiv.innerHTML = `
            <span class="sender">Me to ${selectedGroup.name}:</span>
            <span class="text">${text}</span>
            <span class="time">${new Date().toLocaleTimeString()}</span>
            <span class="sending-indicator"> (sending...)</span>
        `;
        messagesDiv.appendChild(messageDiv);
        messagesDiv.scrollTop = messagesDiv.scrollHeight;

        ws.send(JSON.stringify({
            type: 'group_message',
            payload: { conversation_id: selectedGroup.id, text }
        }));
    }

    input.value = '';
    clearTimeout(typingTimer);
    if (isTyping) {
        isTyping = false;
        sendTypingStatus(false);
    }
}

// ========== СТАТУС ПЕЧАТИ ==========
function sendTypingStatus(typing) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;

    if (currentChatType === 'private' && selectedUser) {
        ws.send(JSON.stringify({
            type: 'typing',
            payload: { to: selectedUser, is_typing: typing }
        }));
    } else if (currentChatType === 'group' && selectedGroup) {
        ws.send(JSON.stringify({
            type: 'typing',
            payload: { in_group: selectedGroup.id, is_typing: typing }
        }));
    }
}

// ========== ЗАПРОСЫ ==========
function getOnlineUsers() {
    if (ws?.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'get_online_users', payload: {} }));
    }
}

function getMyGroups() {
    if (ws?.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'get_my_groups', payload: {} }));
    }
}

// ========== СОЗДАНИЕ ГРУПП ==========
function showCreateGroupForm() {
    document.getElementById('overlay').style.display = 'block';
    document.getElementById('create-group-form').style.display = 'block';
    updateAvailableUsers();
}

function hideCreateGroupForm() {
    document.getElementById('overlay').style.display = 'none';
    document.getElementById('create-group-form').style.display = 'none';
}

function updateAvailableUsers() {
    const container = document.getElementById('available-users');
    if (!container) return;

    container.innerHTML = '';
    allUsers.forEach(user => {
        if (user.username !== currentUser) {
            const div = document.createElement('div');
            div.className = 'checkbox-item';
            div.innerHTML = `<input type="checkbox" value="${user.username}"> ${user.username}`;
            container.appendChild(div);
        }
    });
}

function createGroup() {
    const name = document.getElementById('group-name').value.trim();
    const checkboxes = document.querySelectorAll('#available-users input:checked');
    const members = Array.from(checkboxes).map(cb => cb.value);

    if (!name) {
        alert('Enter group name');
        return;
    }

    if (members.length === 0) {
        alert('Select at least one member');
        return;
    }

    if (ws?.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({
            type: 'create_group',
            payload: { name, members }
        }));
    }

    hideCreateGroupForm();
    document.getElementById('group-name').value = '';
}

// ========== ОБРАБОТЧИКИ СОБЫТИЙ ==========
document.getElementById('message-input').addEventListener('input', function() {
    if (!selectedUser && !selectedGroup) return;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;

    if (this.value.length > 0 && !isTyping) {
        isTyping = true;
        sendTypingStatus(true);
    }

    clearTimeout(typingTimer);
    typingTimer = setTimeout(() => {
        if (isTyping) {
            isTyping = false;
            sendTypingStatus(false);
        }
    }, TYPING_TIMEOUT);
});

document.getElementById('message-input').addEventListener('keypress', (e) => {
    if (e.key === 'Enter') sendMessage();
});
