// GoFreemail JavaScript

// Delete message
function deleteMessage(folder, uid) {
    if (!confirm('Delete this message?')) {
        return;
    }

    fetch('/ajax/delete', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/x-www-form-urlencoded',
        },
        body: `folder=${encodeURIComponent(folder)}&uid=${uid}`
    })
    .then(response => {
        if (response.ok) {
            // Remove row from table
            const row = document.querySelector(`tr[data-uid="${uid}"]`);
            if (row) {
                row.remove();
            }
        } else {
            alert('Failed to delete message');
        }
    })
    .catch(error => {
        console.error('Error:', error);
        alert('Failed to delete message');
    });
}

// Mark message as read
function markRead(folder, uid) {
    fetch('/ajax/mark-read', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/x-www-form-urlencoded',
        },
        body: `folder=${encodeURIComponent(folder)}&uid=${uid}`
    })
    .then(response => {
        if (response.ok) {
            const row = document.querySelector(`tr[data-uid="${uid}"]`);
            if (row) {
                row.classList.remove('unread');
            }
        }
    })
    .catch(error => {
        console.error('Error:', error);
    });
}

// Move message to folder
function moveMessage(srcFolder, uid, destFolder) {
    fetch('/ajax/move', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/x-www-form-urlencoded',
        },
        body: `src_folder=${encodeURIComponent(srcFolder)}&uid=${uid}&dest_folder=${encodeURIComponent(destFolder)}`
    })
    .then(response => {
        if (response.ok) {
            const row = document.querySelector(`tr[data-uid="${uid}"]`);
            if (row) {
                row.remove();
            }
        } else {
            alert('Failed to move message');
        }
    })
    .catch(error => {
        console.error('Error:', error);
        alert('Failed to move message');
    });
}

// Check for new messages (poll status)
function checkStatus() {
    fetch('/ajax/status')
    .then(response => response.json())
    .then(data => {
        // Update unread badge
        const badge = document.querySelector('.folder-list .badge');
        if (badge) {
            if (data.unread > 0) {
                badge.textContent = data.unread;
                badge.style.display = 'inline';
            } else {
                badge.style.display = 'none';
            }
        }
    })
    .catch(error => {
        console.error('Status check error:', error);
    });
}

// Select all messages
function selectAll(checked) {
    document.querySelectorAll('.msg-select').forEach(cb => {
        cb.checked = checked;
    });
}

// Get selected message UIDs
function getSelectedMessages() {
    const selected = [];
    document.querySelectorAll('.msg-select:checked').forEach(cb => {
        selected.push(cb.value);
    });
    return selected;
}

// Delete selected messages
function deleteSelected(folder) {
    const selected = getSelectedMessages();
    if (selected.length === 0) {
        alert('No messages selected');
        return;
    }

    if (!confirm(`Delete ${selected.length} message(s)?`)) {
        return;
    }

    // Delete each selected message
    selected.forEach(uid => {
        deleteMessage(folder, uid);
    });
}

// Keyboard shortcuts
document.addEventListener('keydown', function(e) {
    // Only handle if not in an input field
    if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') {
        return;
    }

    switch(e.key) {
        case 'c':
            // Compose new message
            window.location.href = '/compose';
            break;
        case 'i':
            // Go to inbox
            window.location.href = '/inbox';
            break;
        case 's':
            // Go to sent
            window.location.href = '/sent';
            break;
        case '/':
            // Focus search (if implemented)
            e.preventDefault();
            break;
    }
});

// Auto-refresh status every 60 seconds
setInterval(checkStatus, 60000);

// Initial status check
document.addEventListener('DOMContentLoaded', function() {
    checkStatus();
});
