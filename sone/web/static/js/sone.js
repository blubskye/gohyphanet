// GoSone - JavaScript

// AJAX helper
async function ajax(url, data = {}) {
    const response = await fetch(url, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify(data)
    });
    return response.json();
}

// Quick post submission
async function submitQuickPost(event) {
    event.preventDefault();
    const form = event.target;
    const textarea = form.querySelector('textarea[name="text"]');
    const text = textarea.value.trim();

    if (!text) {
        return false;
    }

    const result = await ajax('/ajax/create-post', { text });

    if (result.success) {
        // Reload to show the new post
        window.location.reload();
    } else {
        showError(result.error || 'Failed to create post');
    }

    return false;
}

// Create post from create-post page
async function submitPost(event) {
    event.preventDefault();
    const form = event.target;
    const textarea = form.querySelector('textarea[name="text"]');
    const recipientInput = form.querySelector('input[name="recipient"]');

    const text = textarea.value.trim();
    const recipient = recipientInput ? recipientInput.value : '';

    if (!text) {
        showError('Please enter some text');
        return false;
    }

    const data = { text };
    if (recipient) {
        data.recipient = recipient;
    }

    const result = await ajax('/ajax/create-post', data);

    if (result.success) {
        window.location.href = '/';
    } else {
        showError(result.error || 'Failed to create post');
    }

    return false;
}

// Toggle like on a post
async function toggleLike(postId) {
    const btn = document.querySelector(`[data-post-id="${postId}"] .btn-like, .post-actions .btn-like`);
    const isLiked = btn && btn.classList.contains('active');

    const endpoint = isLiked ? '/ajax/unlike-post' : '/ajax/like-post';
    const result = await ajax(endpoint, { postId });

    if (result.success) {
        if (btn) {
            btn.classList.toggle('active');
            // Update like count if displayed
            const countSpan = btn.querySelector('.like-count');
            if (countSpan && result.likes !== undefined) {
                countSpan.textContent = result.likes;
            }
        }
    } else {
        showError(result.error || 'Failed to update like');
    }
}

// Toggle like on a reply
async function toggleReplyLike(replyId) {
    const btn = document.querySelector(`[data-reply-id="${replyId}"] .btn-like`);
    const isLiked = btn && btn.classList.contains('active');

    const endpoint = isLiked ? '/ajax/unlike-reply' : '/ajax/like-reply';
    const result = await ajax(endpoint, { replyId });

    if (result.success) {
        if (btn) {
            btn.classList.toggle('active');
        }
    } else {
        showError(result.error || 'Failed to update like');
    }
}

// Show reply form for a post
function showReplyForm(postId) {
    // Hide all other reply forms first
    document.querySelectorAll('.reply-form').forEach(form => {
        form.style.display = 'none';
    });

    // Show the reply form for this post
    const form = document.getElementById(`reply-form-${postId}`);
    if (form) {
        form.style.display = 'block';
        form.querySelector('textarea').focus();
    }
}

// Submit a reply
async function submitReply(event, postId) {
    event.preventDefault();
    const form = event.target;
    const textarea = form.querySelector('textarea');
    const text = textarea.value.trim();

    if (!text) {
        return false;
    }

    const result = await ajax('/ajax/create-reply', { postId, text });

    if (result.success) {
        // Reload to show the new reply
        window.location.reload();
    } else {
        showError(result.error || 'Failed to create reply');
    }

    return false;
}

// Delete a post
async function deletePost(postId) {
    if (!confirm('Are you sure you want to delete this post?')) {
        return;
    }

    const result = await ajax('/ajax/delete-post', { postId });

    if (result.success) {
        // If on post page, go to home; otherwise just reload
        if (window.location.pathname.startsWith('/post/')) {
            window.location.href = '/';
        } else {
            window.location.reload();
        }
    } else {
        showError(result.error || 'Failed to delete post');
    }
}

// Delete a reply
async function deleteReply(replyId) {
    if (!confirm('Are you sure you want to delete this reply?')) {
        return;
    }

    const result = await ajax('/ajax/delete-reply', { replyId });

    if (result.success) {
        window.location.reload();
    } else {
        showError(result.error || 'Failed to delete reply');
    }
}

// Follow a Sone
async function followSone(soneId) {
    const result = await ajax('/ajax/follow', { soneId });

    if (result.success) {
        // Update button state
        const btn = document.querySelector('.btn-follow');
        if (btn) {
            btn.textContent = 'Unfollow';
            btn.onclick = () => unfollowSone(soneId);
            btn.classList.add('following');
        }
    } else {
        showError(result.error || 'Failed to follow Sone');
    }
}

// Unfollow a Sone
async function unfollowSone(soneId) {
    const result = await ajax('/ajax/unfollow', { soneId });

    if (result.success) {
        // Update button state
        const btn = document.querySelector('.btn-follow');
        if (btn) {
            btn.textContent = 'Follow';
            btn.onclick = () => followSone(soneId);
            btn.classList.remove('following');
        }
    } else {
        showError(result.error || 'Failed to unfollow Sone');
    }
}

// Trust a Sone (set explicit trust)
async function trustSone(soneId) {
    const trust = prompt('Enter trust value (-100 to 100):', '75');
    if (trust === null) return;

    const trustValue = parseInt(trust, 10);
    if (isNaN(trustValue) || trustValue < -100 || trustValue > 100) {
        showError('Trust value must be between -100 and 100');
        return;
    }

    const comment = prompt('Enter trust comment (optional):', '');

    const result = await ajax('/ajax/trust', {
        soneId,
        trust: trustValue,
        comment: comment || ''
    });

    if (result.success) {
        showSuccess('Trust updated successfully');
    } else {
        showError(result.error || 'Failed to update trust');
    }
}

// Distrust a Sone
async function distrustSone(soneId) {
    if (!confirm('Are you sure you want to distrust this Sone? This will hide their posts.')) {
        return;
    }

    const result = await ajax('/ajax/distrust', { soneId });

    if (result.success) {
        showSuccess('Sone distrusted');
        window.location.reload();
    } else {
        showError(result.error || 'Failed to distrust Sone');
    }
}

// Bookmark a post
async function bookmarkPost(postId) {
    const result = await ajax('/ajax/bookmark', { postId });

    if (result.success) {
        const btn = document.querySelector(`[data-post-id="${postId}"] .btn-bookmark, .post-actions .btn-bookmark`);
        if (btn) {
            btn.classList.add('active');
            btn.onclick = () => unbookmarkPost(postId);
            btn.textContent = 'Bookmarked';
        }
    } else {
        showError(result.error || 'Failed to bookmark post');
    }
}

// Remove bookmark
async function unbookmarkPost(postId) {
    const result = await ajax('/ajax/unbookmark', { postId });

    if (result.success) {
        const btn = document.querySelector(`[data-post-id="${postId}"] .btn-bookmark, .post-actions .btn-bookmark`);
        if (btn) {
            btn.classList.remove('active');
            btn.onclick = () => bookmarkPost(postId);
            btn.textContent = 'Bookmark';
        }
        // If on bookmarks page, remove the post from view
        if (window.location.pathname === '/bookmarks') {
            const postEl = document.querySelector(`[data-post-id="${postId}"]`);
            if (postEl) {
                postEl.remove();
            }
        }
    } else {
        showError(result.error || 'Failed to remove bookmark');
    }
}

// Dismiss a notification
async function dismissNotification(id) {
    const result = await ajax('/ajax/dismiss-notification', { id });

    if (result.success) {
        const notification = document.querySelector(`[data-notification-id="${id}"]`);
        if (notification) {
            notification.remove();
        }
    }
}

// Dismiss all notifications
async function dismissAllNotifications() {
    const result = await ajax('/ajax/dismiss-all-notifications', {});

    if (result.success) {
        document.querySelectorAll('.notification').forEach(el => el.remove());
    }
}

// Show error message
function showError(message) {
    showAlert(message, 'error');
}

// Show success message
function showSuccess(message) {
    showAlert(message, 'success');
}

// Show alert
function showAlert(message, type = 'info') {
    // Remove existing alerts
    document.querySelectorAll('.alert-toast').forEach(el => el.remove());

    const alert = document.createElement('div');
    alert.className = `alert alert-${type} alert-toast`;
    alert.textContent = message;
    alert.style.cssText = `
        position: fixed;
        top: 80px;
        right: 20px;
        z-index: 1000;
        animation: slideIn 0.3s ease;
    `;

    document.body.appendChild(alert);

    // Auto-dismiss after 5 seconds
    setTimeout(() => {
        alert.style.animation = 'slideOut 0.3s ease';
        setTimeout(() => alert.remove(), 300);
    }, 5000);
}

// Load notifications periodically
function startNotificationPolling() {
    setInterval(async () => {
        try {
            const result = await ajax('/ajax/notifications', {});
            if (result.success && result.data) {
                updateNotificationBadge(result.data.count || 0);
            }
        } catch (e) {
            // Silently ignore polling errors
        }
    }, 30000); // Every 30 seconds
}

// Update notification badge in nav
function updateNotificationBadge(count) {
    let badge = document.querySelector('.notification-badge');
    if (count > 0) {
        if (!badge) {
            badge = document.createElement('span');
            badge.className = 'notification-badge';
            const navUser = document.querySelector('.nav-user');
            if (navUser) {
                navUser.prepend(badge);
            }
        }
        badge.textContent = count;
    } else if (badge) {
        badge.remove();
    }
}

// Initialize on page load
document.addEventListener('DOMContentLoaded', () => {
    // Start polling for notifications
    startNotificationPolling();

    // Add keyboard shortcuts
    document.addEventListener('keydown', (e) => {
        // Escape to close reply forms
        if (e.key === 'Escape') {
            document.querySelectorAll('.reply-form').forEach(form => {
                form.style.display = 'none';
            });
        }

        // n to focus on quick post
        if (e.key === 'n' && !e.target.matches('input, textarea')) {
            const quickPost = document.querySelector('.quick-post textarea');
            if (quickPost) {
                e.preventDefault();
                quickPost.focus();
            }
        }
    });
});

// Add CSS animation styles
const style = document.createElement('style');
style.textContent = `
    @keyframes slideIn {
        from { transform: translateX(100%); opacity: 0; }
        to { transform: translateX(0); opacity: 1; }
    }
    @keyframes slideOut {
        from { transform: translateX(0); opacity: 1; }
        to { transform: translateX(100%); opacity: 0; }
    }
    .notification-badge {
        background: var(--error-color);
        color: white;
        border-radius: 50%;
        padding: 0.25rem 0.5rem;
        font-size: 0.75rem;
        font-weight: bold;
        margin-right: 0.5rem;
    }
    .btn-follow.following {
        background: var(--secondary-color);
    }
`;
document.head.appendChild(style);
