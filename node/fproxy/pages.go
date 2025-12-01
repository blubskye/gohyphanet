package fproxy

import (
	"fmt"
	"net/http"
	"runtime"
)

// serveConfigPage serves the configuration page
// Based on fred-next's ConfigToadlet.java (GPL v2+)
func (s *FProxyServer) serveConfigPage(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Configuration - Freenet</title>
    <style>
        * { margin: 0; padding: 0; }
        body { font-family: Arial, sans-serif; font-size: 11pt; background-color: #f5f5f5; }
        #topbar { background: linear-gradient(to bottom, #667eea 0%, #764ba2 100%); border-bottom: 1px solid #ccc; min-height: 50px; padding: 0.333em; color: white; }
        #topbar h1 { font-size: 1.818em; font-weight: normal; padding-top: 0.35em; text-align: center; color: white; }
        #navbar { float: left; position: relative; left: 0.667em; top: 0.667em; width: 11.081em; border: 1px solid #ccc; background: #fff; }
        #navlist { list-style-type: none; }
        #navlist li { border-bottom: 1px dotted #ddd; }
        #navlist a { display: block; padding: 0.667em; text-decoration: none; color: #333; font-weight: bold; }
        #navlist a:hover { background-color: #f0f0f0; }
        #navlist li.navlist-selected a { background-color: #667eea; color: white; }
        #content { margin-top: 0.667em; margin-left: 12.5em; margin-right: 0.667em; position: relative; }
        div.infobox { background: #fff; border: 1px solid #ccc; margin-bottom: 0.667em; border-radius: 4px; }
        div.infobox-header { background: #f0f0f0; padding: 0.667em; border-bottom: 1px dotted #ccc; font-weight: bold; }
        div.infobox-content { padding: 0.667em; }
        div.infobox-information { border-left: 4px solid #3498db; }
        div.infobox-warning { border-left: 4px solid #f39c12; }
        .config-table { width: 100%; border-collapse: collapse; margin-top: 10px; }
        .config-table th, .config-table td { padding: 10px; text-align: left; border-bottom: 1px solid #eee; }
        .config-table th { background: #f9f9f9; font-weight: bold; width: 40%; }
        .config-table tr:hover { background: #fafafa; }
        .config-value { font-family: monospace; color: #2c3e50; }
    </style>
</head>
<body>
    <div id="topbar"><h1>Freenet</h1></div>
    <div id="navbar">
        <ul id="navlist">
            <li><a href="/">Browse</a></li>
            <li><a href="/queue/">Queue</a></li>
            <li><a href="/uploads/">Uploads</a></li>
            <li><a href="/downloads/">Downloads</a></li>
            <li class="navlist-selected"><a href="/config/">Configuration</a></li>
            <li><a href="/plugins/">Plugins</a></li>
            <li><a href="/friends/">Friends</a></li>
            <li><a href="/strangers/">Strangers</a></li>
            <li><a href="/alerts/">Alerts</a></li>
            <li><a href="/statistics/">Statistics</a></li>
        </ul>
    </div>
    <div id="content">
        <div class="infobox infobox-information">
            <div class="infobox-header">Node Configuration</div>
            <div class="infobox-content">
                <p>Current node configuration settings (read-only):</p>
                <table class="config-table">
                    <tr><th>Setting</th><th>Value</th></tr>
                    <tr><td>Node Version</td><td class="config-value">GoHyphanet 0.1.0</td></tr>
                    <tr><td>Protocol Version</td><td class="config-value">Freenet 0.7 Compatible</td></tr>
                    <tr><td>FProxy Port</td><td class="config-value">8888</td></tr>
                    <tr><td>FCP Port</td><td class="config-value">9481</td></tr>
                    <tr><td>FProxy Bind Address</td><td class="config-value">127.0.0.1</td></tr>
                    <tr><td>FCP Bind Address</td><td class="config-value">127.0.0.1</td></tr>
                    <tr><td>Node Location</td><td class="config-value">0.5</td></tr>
                    <tr><td>Max Content Size</td><td class="config-value">100 MB</td></tr>
                </table>
            </div>
        </div>

        <div class="infobox infobox-information">
            <div class="infobox-header">Runtime Information</div>
            <div class="infobox-content">
                <table class="config-table">
                    <tr><th>Property</th><th>Value</th></tr>
                    <tr><td>Operating System</td><td class="config-value">` + runtime.GOOS + `</td></tr>
                    <tr><td>Architecture</td><td class="config-value">` + runtime.GOARCH + `</td></tr>
                    <tr><td>Go Version</td><td class="config-value">` + runtime.Version() + `</td></tr>
                    <tr><td>CPUs</td><td class="config-value">` + fmt.Sprintf("%d", runtime.NumCPU()) + `</td></tr>
                </table>
            </div>
        </div>

        <div class="infobox infobox-warning">
            <div class="infobox-header">Implementation Status</div>
            <div class="infobox-content">
                <p><strong>Note:</strong> Full configuration editing is under development.</p>
                <p style="margin-top: 10px;">Future versions will support:</p>
                <ul style="margin-left: 20px; margin-top: 10px;">
                    <li>Web-based configuration editing</li>
                    <li>Port and bind address changes</li>
                    <li>Bandwidth and storage limits</li>
                    <li>Security and privacy settings</li>
                    <li>Plugin configuration</li>
                </ul>
            </div>
        </div>

        <div class="infobox infobox-warning">
            <div class="infobox-header">Node Control</div>
            <div class="infobox-content">
                <p><strong>Shutdown Node</strong></p>
                <p style="margin-top: 10px;">Click the button below to gracefully shut down the Freenet node.</p>
                <form method="GET" action="/shutdown/" style="margin-top: 15px;">
                    <input type="submit" value="Shutdown Node" style="padding: 10px 20px; background: #e74c3c; color: white; border: none; border-radius: 3px; cursor: pointer; font-size: 14px;">
                </form>
            </div>
        </div>
    </div>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Powered-By", "GoHyphanet/0.1")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, html)
}

// serveAlertsPage serves the alerts page
// Based on fred-next's alerts system
func (s *FProxyServer) serveAlertsPage(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Alerts - Freenet</title>
    <style>
        * { margin: 0; padding: 0; }
        body { font-family: Arial, sans-serif; font-size: 11pt; background-color: #f5f5f5; }
        #topbar { background: linear-gradient(to bottom, #667eea 0%, #764ba2 100%); border-bottom: 1px solid #ccc; min-height: 50px; padding: 0.333em; color: white; }
        #topbar h1 { font-size: 1.818em; font-weight: normal; padding-top: 0.35em; text-align: center; color: white; }
        #navbar { float: left; position: relative; left: 0.667em; top: 0.667em; width: 11.081em; border: 1px solid #ccc; background: #fff; }
        #navlist { list-style-type: none; }
        #navlist li { border-bottom: 1px dotted #ddd; }
        #navlist a { display: block; padding: 0.667em; text-decoration: none; color: #333; font-weight: bold; }
        #navlist a:hover { background-color: #f0f0f0; }
        #navlist li.navlist-selected a { background-color: #667eea; color: white; }
        #content { margin-top: 0.667em; margin-left: 12.5em; margin-right: 0.667em; position: relative; }
        div.infobox { background: #fff; border: 1px solid #ccc; margin-bottom: 0.667em; border-radius: 4px; }
        div.infobox-header { background: #f0f0f0; padding: 0.667em; border-bottom: 1px dotted #ccc; font-weight: bold; }
        div.infobox-content { padding: 0.667em; }
        div.infobox-information { border-left: 4px solid #3498db; }
        div.infobox-success { border-left: 4px solid #27ae60; }
        .alert-item { padding: 15px; margin: 10px 0; background: #f9f9f9; border-radius: 4px; border-left: 3px solid #3498db; }
        .alert-time { color: #7f8c8d; font-size: 0.9em; }
        .no-alerts { text-align: center; padding: 30px; color: #95a5a6; }
    </style>
</head>
<body>
    <div id="topbar"><h1>Freenet</h1></div>
    <div id="navbar">
        <ul id="navlist">
            <li><a href="/">Browse</a></li>
            <li><a href="/queue/">Queue</a></li>
            <li><a href="/uploads/">Uploads</a></li>
            <li><a href="/downloads/">Downloads</a></li>
            <li><a href="/config/">Configuration</a></li>
            <li><a href="/plugins/">Plugins</a></li>
            <li><a href="/friends/">Friends</a></li>
            <li><a href="/strangers/">Strangers</a></li>
            <li class="navlist-selected"><a href="/alerts/">Alerts</a></li>
            <li><a href="/statistics/">Statistics</a></li>
        </ul>
    </div>
    <div id="content">
        <div class="infobox infobox-success">
            <div class="infobox-header">System Alerts</div>
            <div class="infobox-content">
                <div class="no-alerts">
                    <p><strong>No active alerts</strong></p>
                    <p style="margin-top: 10px;">Your node is running normally with no warnings or errors.</p>
                </div>
            </div>
        </div>

        <div class="infobox infobox-information">
            <div class="infobox-header">About Alerts</div>
            <div class="infobox-content">
                <p>The alerts system monitors your node and notifies you of:</p>
                <ul style="margin-left: 20px; margin-top: 10px;">
                    <li>Critical errors that require attention</li>
                    <li>Configuration problems</li>
                    <li>Network connectivity issues</li>
                    <li>Storage space warnings</li>
                    <li>Security notifications</li>
                    <li>Update availability</li>
                </ul>
                <p style="margin-top: 15px;">
                    <strong>Implementation status:</strong> Alert tracking is under development.
                    Future versions will include real-time monitoring and notification features.
                </p>
            </div>
        </div>
    </div>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Powered-By", "GoHyphanet/0.1")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, html)
}

// serveStatisticsPage serves the statistics page
// Based on fred-next's StatisticsToadlet.java
func (s *FProxyServer) serveStatisticsPage(w http.ResponseWriter, r *http.Request) {
	stats := s.GetStats()

	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Statistics - Freenet</title>
    <style>
        * { margin: 0; padding: 0; }
        body { font-family: Arial, sans-serif; font-size: 11pt; background-color: #f5f5f5; }
        #topbar { background: linear-gradient(to bottom, #667eea 0%, #764ba2 100%); border-bottom: 1px solid #ccc; min-height: 50px; padding: 0.333em; color: white; }
        #topbar h1 { font-size: 1.818em; font-weight: normal; padding-top: 0.35em; text-align: center; color: white; }
        #navbar { float: left; position: relative; left: 0.667em; top: 0.667em; width: 11.081em; border: 1px solid #ccc; background: #fff; }
        #navlist { list-style-type: none; }
        #navlist li { border-bottom: 1px dotted #ddd; }
        #navlist a { display: block; padding: 0.667em; text-decoration: none; color: #333; font-weight: bold; }
        #navlist a:hover { background-color: #f0f0f0; }
        #navlist li.navlist-selected a { background-color: #667eea; color: white; }
        #content { margin-top: 0.667em; margin-left: 12.5em; margin-right: 0.667em; position: relative; }
        div.infobox { background: #fff; border: 1px solid #ccc; margin-bottom: 0.667em; border-radius: 4px; }
        div.infobox-header { background: #f0f0f0; padding: 0.667em; border-bottom: 1px dotted #ccc; font-weight: bold; }
        div.infobox-content { padding: 0.667em; }
        div.infobox-information { border-left: 4px solid #3498db; }
        .stats-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 15px; margin-top: 15px; }
        .stat-box { background: #f9f9f9; padding: 15px; border-radius: 4px; border-left: 3px solid #3498db; }
        .stat-label { font-weight: bold; color: #7f8c8d; font-size: 0.9em; margin-bottom: 5px; }
        .stat-value { font-size: 1.5em; color: #2c3e50; font-weight: bold; }
        .stats-table { width: 100%; border-collapse: collapse; margin-top: 10px; }
        .stats-table th, .stats-table td { padding: 10px; text-align: left; border-bottom: 1px solid #eee; }
        .stats-table th { background: #f9f9f9; font-weight: bold; }
    </style>
</head>
<body>
    <div id="topbar"><h1>Freenet</h1></div>
    <div id="navbar">
        <ul id="navlist">
            <li><a href="/">Browse</a></li>
            <li><a href="/queue/">Queue</a></li>
            <li><a href="/uploads/">Uploads</a></li>
            <li><a href="/downloads/">Downloads</a></li>
            <li><a href="/config/">Configuration</a></li>
            <li><a href="/plugins/">Plugins</a></li>
            <li><a href="/friends/">Friends</a></li>
            <li><a href="/strangers/">Strangers</a></li>
            <li><a href="/alerts/">Alerts</a></li>
            <li class="navlist-selected"><a href="/statistics/">Statistics</a></li>
        </ul>
    </div>
    <div id="content">
        <div class="infobox infobox-information">
            <div class="infobox-header">FProxy Statistics</div>
            <div class="infobox-content">
                <div class="stats-grid">
                    <div class="stat-box">
                        <div class="stat-label">Total Requests</div>
                        <div class="stat-value">` + fmt.Sprintf("%d", stats.RequestCount) + `</div>
                    </div>
                    <div class="stat-box">
                        <div class="stat-label">Errors</div>
                        <div class="stat-value">` + fmt.Sprintf("%d", stats.ErrorCount) + `</div>
                    </div>
                </div>
                <p style="margin-top: 15px;"><strong>Status:</strong> ` + (map[bool]string{true: "Enabled", false: "Disabled"}[stats.Enabled]) + `</p>
                <p><strong>Listen Address:</strong> ` + stats.ListenAddr + `</p>
            </div>
        </div>

        <div class="infobox infobox-information">
            <div class="infobox-header">Network Statistics</div>
            <div class="infobox-content">
                <table class="stats-table">
                    <tr><th>Metric</th><th>Value</th></tr>
                    <tr><td>Connected Peers</td><td>0</td></tr>
                    <tr><td>Total Peers</td><td>0</td></tr>
                    <tr><td>Backed Off Peers</td><td>0</td></tr>
                    <tr><td>Too New Peers</td><td>0</td></tr>
                    <tr><td>Too Old Peers</td><td>0</td></tr>
                    <tr><td>Disconnected Peers</td><td>0</td></tr>
                </table>
                <p style="margin-top: 15px; color: #7f8c8d; font-size: 0.9em;">
                    <em>Note: Peer connection statistics will be available when the networking layer is fully implemented.</em>
                </p>
            </div>
        </div>

        <div class="infobox infobox-information">
            <div class="infobox-header">Datastore Statistics</div>
            <div class="infobox-content">
                <table class="stats-table">
                    <tr><th>Metric</th><th>Value</th></tr>
                    <tr><td>Blocks Stored</td><td>N/A</td></tr>
                    <tr><td>Cache Hits</td><td>N/A</td></tr>
                    <tr><td>Cache Misses</td><td>N/A</td></tr>
                    <tr><td>Store Size</td><td>N/A</td></tr>
                </table>
                <p style="margin-top: 15px; color: #7f8c8d; font-size: 0.9em;">
                    <em>Note: Detailed datastore statistics tracking is under development.</em>
                </p>
            </div>
        </div>

        <div class="infobox infobox-information">
            <div class="infobox-header">Runtime Statistics</div>
            <div class="infobox-content">
                <table class="stats-table">
                    <tr><th>Property</th><th>Value</th></tr>
                    <tr><td>Go Routines</td><td>` + fmt.Sprintf("%d", runtime.NumGoroutine()) + `</td></tr>
                    <tr><td>Available CPUs</td><td>` + fmt.Sprintf("%d", runtime.NumCPU()) + `</td></tr>
                </table>
            </div>
        </div>
    </div>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Powered-By", "GoHyphanet/0.1")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, html)
}

// servePluginsPage serves the plugins page
func (s *FProxyServer) servePluginsPage(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Plugins - Freenet</title>
    <style>
        * { margin: 0; padding: 0; }
        body { font-family: Arial, sans-serif; font-size: 11pt; background-color: #f5f5f5; }
        #topbar { background: linear-gradient(to bottom, #667eea 0%, #764ba2 100%); border-bottom: 1px solid #ccc; min-height: 50px; padding: 0.333em; color: white; }
        #topbar h1 { font-size: 1.818em; font-weight: normal; padding-top: 0.35em; text-align: center; color: white; }
        #navbar { float: left; position: relative; left: 0.667em; top: 0.667em; width: 11.081em; border: 1px solid #ccc; background: #fff; }
        #navlist { list-style-type: none; }
        #navlist li { border-bottom: 1px dotted #ddd; }
        #navlist a { display: block; padding: 0.667em; text-decoration: none; color: #333; font-weight: bold; }
        #navlist a:hover { background-color: #f0f0f0; }
        #navlist li.navlist-selected a { background-color: #667eea; color: white; }
        #content { margin-top: 0.667em; margin-left: 12.5em; margin-right: 0.667em; position: relative; }
        div.infobox { background: #fff; border: 1px solid #ccc; margin-bottom: 0.667em; border-radius: 4px; }
        div.infobox-header { background: #f0f0f0; padding: 0.667em; border-bottom: 1px dotted #ccc; font-weight: bold; }
        div.infobox-content { padding: 0.667em; }
        div.infobox-information { border-left: 4px solid #3498db; }
        .no-plugins { text-align: center; padding: 30px; color: #95a5a6; }
    </style>
</head>
<body>
    <div id="topbar"><h1>Freenet</h1></div>
    <div id="navbar">
        <ul id="navlist">
            <li><a href="/">Browse</a></li>
            <li><a href="/queue/">Queue</a></li>
            <li><a href="/uploads/">Uploads</a></li>
            <li><a href="/downloads/">Downloads</a></li>
            <li><a href="/config/">Configuration</a></li>
            <li class="navlist-selected"><a href="/plugins/">Plugins</a></li>
            <li><a href="/friends/">Friends</a></li>
            <li><a href="/strangers/">Strangers</a></li>
            <li><a href="/alerts/">Alerts</a></li>
            <li><a href="/statistics/">Statistics</a></li>
        </ul>
    </div>
    <div id="content">
        <div class="infobox infobox-information">
            <div class="infobox-header">Installed Plugins</div>
            <div class="infobox-content">
                <div class="no-plugins">
                    <p><strong>No plugins installed</strong></p>
                    <p style="margin-top: 10px;">The plugin system is under development.</p>
                </div>
            </div>
        </div>

        <div class="infobox infobox-information">
            <div class="infobox-header">About Plugins</div>
            <div class="infobox-content">
                <p>Freenet plugins extend the functionality of your node with additional features:</p>
                <ul style="margin-left: 20px; margin-top: 10px;">
                    <li><strong>Library</strong> - Browse and search freesite indexes</li>
                    <li><strong>Sone</strong> - Decentralized social networking</li>
                    <li><strong>FMS</strong> - Freenet Message System (forums)</li>
                    <li><strong>Sharesite</strong> - File sharing plugin</li>
                    <li><strong>WebOfTrust</strong> - Decentralized identity and trust system</li>
                </ul>
                <p style="margin-top: 15px;">
                    <strong>Implementation status:</strong> The plugin system is under development.
                    Future versions will support loading and managing Freenet plugins.
                </p>
            </div>
        </div>
    </div>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Powered-By", "GoHyphanet/0.1")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, html)
}

// servePeersPage serves the friends/peers page
func (s *FProxyServer) servePeersPage(w http.ResponseWriter, r *http.Request, peerType string) {
	pageTitle := "Friends"
	if peerType == "strangers" {
		pageTitle = "Strangers"
	}

	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>` + pageTitle + ` - Freenet</title>
    <style>
        * { margin: 0; padding: 0; }
        body { font-family: Arial, sans-serif; font-size: 11pt; background-color: #f5f5f5; }
        #topbar { background: linear-gradient(to bottom, #667eea 0%, #764ba2 100%); border-bottom: 1px solid #ccc; min-height: 50px; padding: 0.333em; color: white; }
        #topbar h1 { font-size: 1.818em; font-weight: normal; padding-top: 0.35em; text-align: center; color: white; }
        #navbar { float: left; position: relative; left: 0.667em; top: 0.667em; width: 11.081em; border: 1px solid #ccc; background: #fff; }
        #navlist { list-style-type: none; }
        #navlist li { border-bottom: 1px dotted #ddd; }
        #navlist a { display: block; padding: 0.667em; text-decoration: none; color: #333; font-weight: bold; }
        #navlist a:hover { background-color: #f0f0f0; }
        #navlist li.navlist-selected a { background-color: #667eea; color: white; }
        #content { margin-top: 0.667em; margin-left: 12.5em; margin-right: 0.667em; position: relative; }
        div.infobox { background: #fff; border: 1px solid #ccc; margin-bottom: 0.667em; border-radius: 4px; }
        div.infobox-header { background: #f0f0f0; padding: 0.667em; border-bottom: 1px dotted #ccc; font-weight: bold; }
        div.infobox-content { padding: 0.667em; }
        div.infobox-information { border-left: 4px solid #3498db; }
        .no-peers { text-align: center; padding: 30px; color: #95a5a6; }
    </style>
</head>
<body>
    <div id="topbar"><h1>Freenet</h1></div>
    <div id="navbar">
        <ul id="navlist">
            <li><a href="/">Browse</a></li>
            <li><a href="/queue/">Queue</a></li>
            <li><a href="/uploads/">Uploads</a></li>
            <li><a href="/downloads/">Downloads</a></li>
            <li><a href="/config/">Configuration</a></li>
            <li><a href="/plugins/">Plugins</a></li>
            <li` + (map[bool]string{true: ` class="navlist-selected"`, false: ``}[peerType == "friends"]) + `><a href="/friends/">Friends</a></li>
            <li` + (map[bool]string{true: ` class="navlist-selected"`, false: ``}[peerType == "strangers"]) + `><a href="/strangers/">Strangers</a></li>
            <li><a href="/alerts/">Alerts</a></li>
            <li><a href="/statistics/">Statistics</a></li>
        </ul>
    </div>
    <div id="content">
        <div class="infobox infobox-information">
            <div class="infobox-header">` + pageTitle + ` (0 peers)</div>
            <div class="infobox-content">
                <div class="no-peers">
                    <p><strong>No ` + peerType + ` connected</strong></p>
                    <p style="margin-top: 10px;">Peer connections will appear here once the networking layer is active.</p>
                </div>
            </div>
        </div>

        <div class="infobox infobox-information">
            <div class="infobox-header">About ` + pageTitle + `</div>
            <div class="infobox-content">
                <p>` + (map[bool]string{
		true:  "Friends are peers you have manually added and trust. They form your darknet connections.",
		false: "Strangers are peers discovered through opennet (random peer discovery).",
	}[peerType == "friends"]) + `</p>
                <ul style="margin-left: 20px; margin-top: 10px;">
                    <li>View peer connection status and statistics</li>
                    <li>Monitor data transfer rates</li>
                    <li>Check peer locations and routing performance</li>
                    <li>Manage peer connections</li>
                </ul>
                <p style="margin-top: 15px;">
                    <strong>Implementation status:</strong> The networking and peer management system is under development.
                </p>
            </div>
        </div>
    </div>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Powered-By", "GoHyphanet/0.1")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, html)
}
