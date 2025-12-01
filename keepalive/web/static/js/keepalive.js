// GoKeepalive JavaScript

// Update status periodically
function updateStatus() {
    fetch('/ajax/status')
        .then(response => response.json())
        .then(data => {
            // Update progress bar if present
            const progressFill = document.getElementById('progress-fill');
            const progressText = document.getElementById('progress-text');

            if (progressFill && data.percent !== undefined) {
                progressFill.style.width = data.percent + '%';
            }

            if (progressText && data.segment !== undefined) {
                progressText.textContent = `Segment ${data.segment + 1} of ${data.total_segments}`;
            }

            // Reload page if state changed to idle (reinsertion complete)
            const currentState = document.querySelector('.status-value');
            if (currentState && currentState.textContent !== data.state) {
                if (data.state === 'idle') {
                    location.reload();
                }
            }
        })
        .catch(error => {
            console.error('Status update error:', error);
        });
}

// Update progress stats
function updateProgress() {
    fetch('/ajax/progress')
        .then(response => response.json())
        .then(data => {
            if (data.running) {
                // Could update live stats here
            }
        })
        .catch(error => {
            console.error('Progress update error:', error);
        });
}

// Start periodic updates if on dashboard with active reinsertion
document.addEventListener('DOMContentLoaded', function() {
    const statusValue = document.querySelector('.status-value');
    if (statusValue && (statusValue.classList.contains('status-running') || statusValue.classList.contains('status-paused'))) {
        // Update every 2 seconds during active reinsertion
        setInterval(updateStatus, 2000);
        setInterval(updateProgress, 5000);
    }
});

// Confirm dialogs
document.querySelectorAll('form[data-confirm]').forEach(form => {
    form.addEventListener('submit', function(e) {
        if (!confirm(this.dataset.confirm)) {
            e.preventDefault();
        }
    });
});

// Auto-refresh site list
function refreshSites() {
    const table = document.querySelector('.sites-list table tbody');
    if (!table) return;

    fetch('/ajax/sites')
        .then(response => response.json())
        .then(sites => {
            // Update table rows
            sites.forEach(site => {
                const row = document.querySelector(`tr[data-site-id="${site.id}"]`);
                if (row) {
                    const stateCell = row.querySelector('.site-state');
                    const availCell = row.querySelector('.site-availability');
                    if (stateCell) stateCell.textContent = site.state;
                    if (availCell) availCell.textContent = site.availability.toFixed(1) + '%';
                }
            });
        })
        .catch(error => {
            console.error('Sites refresh error:', error);
        });
}

// Keyboard shortcuts
document.addEventListener('keydown', function(e) {
    // Only handle if not in an input field
    if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') {
        return;
    }

    switch(e.key) {
        case 'h':
            // Go to home
            window.location.href = '/';
            break;
        case 's':
            // Go to sites
            window.location.href = '/sites';
            break;
        case 'a':
            // Add site
            window.location.href = '/add';
            break;
        case 't':
            // Stats
            window.location.href = '/stats';
            break;
    }
});

// Tooltip support
document.querySelectorAll('[data-tooltip]').forEach(el => {
    el.addEventListener('mouseenter', function() {
        const tip = document.createElement('div');
        tip.className = 'tooltip';
        tip.textContent = this.dataset.tooltip;
        document.body.appendChild(tip);

        const rect = this.getBoundingClientRect();
        tip.style.left = rect.left + 'px';
        tip.style.top = (rect.bottom + 5) + 'px';

        this._tooltip = tip;
    });

    el.addEventListener('mouseleave', function() {
        if (this._tooltip) {
            this._tooltip.remove();
            this._tooltip = null;
        }
    });
});

// Copy URI to clipboard
function copyURI(uri) {
    navigator.clipboard.writeText(uri).then(() => {
        alert('URI copied to clipboard');
    }).catch(err => {
        console.error('Failed to copy:', err);
    });
}
