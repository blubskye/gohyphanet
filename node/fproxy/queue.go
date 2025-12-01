package fproxy

import (
	"fmt"
	"net/http"
)

// serveQueuePage serves the downloads queue page
// Based on fred-next's QueueToadlet.java (GPL v2+)
// Ported from: freenet/clients/http/QueueToadlet.java
func (s *FProxyServer) serveQueuePage(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Queue - Freenet</title>
    <style>
        /* Based on Freenet/Hyphanet default theme */

        * {
            margin: 0;
            padding: 0;
        }

        body {
            font-family: Arial, sans-serif;
            font-size: 11pt;
            margin: 0;
            padding: 0;
            background-color: #f5f5f5;
        }

        /* Top Title Bar */
        #topbar {
            background: linear-gradient(to bottom, #667eea 0%, #764ba2 100%);
            border-bottom: 1px solid #ccc;
            min-height: 50px;
            padding: 0.333em;
            color: white;
        }

        #topbar h1 {
            font-size: 1.818em;
            font-weight: normal;
            padding-top: 0.35em;
            text-align: center;
            color: white;
        }

        /* Navigation Sidebar */
        #navbar {
            float: left;
            position: relative;
            left: 0.667em;
            top: 0.667em;
            width: 11.081em;
            border: 1px solid #ccc;
            background: #fff;
        }

        #navlist {
            list-style-type: none;
            margin: 0;
            padding: 0;
        }

        #navlist li {
            border-bottom: 1px dotted #ddd;
        }

        #navlist a {
            display: block;
            padding: 0.667em;
            text-decoration: none;
            color: #333;
            font-weight: bold;
        }

        #navlist a:hover {
            background-color: #f0f0f0;
        }

        #navlist li.navlist-selected a {
            background-color: #667eea;
            color: white;
        }

        /* Main Content Area */
        #content {
            margin-top: 0.667em;
            margin-left: 12.5em;
            margin-right: 0.667em;
            position: relative;
        }

        /* Infoboxes */
        div.infobox {
            background: #fff;
            border: 1px solid #ccc;
            margin-bottom: 0.667em;
            border-radius: 4px;
        }

        div.infobox-header {
            background: #f0f0f0;
            padding: 0.667em;
            border-bottom: 1px dotted #ccc;
            font-weight: bold;
        }

        div.infobox-content {
            padding: 0.667em;
        }

        div.infobox-information {
            border-left: 4px solid #3498db;
        }

        /* Queue-specific styles */
        .queue-empty-message {
            padding: 20px;
            text-align: center;
            color: #666;
        }

        .queue-help {
            margin-top: 15px;
            padding: 15px;
            background: #f9f9f9;
            border-radius: 4px;
            border-left: 3px solid #3498db;
        }

        .queue-help h3 {
            margin-bottom: 10px;
            color: #333;
        }

        .queue-help ul {
            margin-left: 20px;
            margin-top: 10px;
        }

        .queue-help li {
            margin: 5px 0;
        }

        code {
            background: #f0f0f0;
            padding: 2px 6px;
            border-radius: 3px;
            font-family: monospace;
        }
    </style>
</head>
<body>
    <!-- Top Bar -->
    <div id="topbar">
        <h1>Freenet</h1>
    </div>

    <!-- Navigation Sidebar -->
    <div id="navbar">
        <ul id="navlist">
            <li><a href="/" title="Browse Freenet">Browse</a></li>
            <li class="navlist-selected"><a href="/queue/" title="View downloads queue">Queue</a></li>
            <li><a href="/uploads/" title="View uploads">Uploads</a></li>
            <li><a href="/downloads/" title="Completed downloads">Downloads</a></li>
            <li><a href="/config/" title="Configuration">Configuration</a></li>
            <li><a href="/plugins/" title="Plugins">Plugins</a></li>
            <li><a href="/friends/" title="Friends">Friends</a></li>
            <li><a href="/strangers/" title="Strangers">Strangers</a></li>
            <li><a href="/alerts/" title="Alerts">Alerts</a></li>
            <li><a href="/statistics/" title="Statistics">Statistics</a></li>
        </ul>
    </div>

    <!-- Main Content -->
    <div id="content">
        <!-- Empty Queue Message -->
        <div class="infobox infobox-information">
            <div class="infobox-header">Downloads Queue (0/0/0)</div>
            <div class="infobox-content">
                <div class="queue-empty-message">
                    <strong>Global queue is empty</strong>
                    <p style="margin-top: 10px;">There are no tasks in the global queue.</p>
                </div>

                <div class="queue-help">
                    <h3>About the Queue</h3>
                    <p>The downloads queue shows all active and completed download requests from the Freenet network.</p>

                    <h3 style="margin-top: 15px;">How to add downloads:</h3>
                    <ul>
                        <li>Use the <a href="/">Browse page</a> to fetch content by Freenet URI</li>
                        <li>Use the FCP protocol (port 9481) to programmatically queue downloads</li>
                        <li>Downloads initiated through FProxy will appear here when request tracking is enabled</li>
                    </ul>

                    <h3 style="margin-top: 15px;">Queue sections:</h3>
                    <ul>
                        <li><strong>In Progress</strong> - Downloads currently being fetched</li>
                        <li><strong>Failed</strong> - Downloads that encountered errors</li>
                        <li><strong>Completed</strong> - Successfully downloaded content</li>
                    </ul>
                </div>
            </div>
        </div>

        <!-- Development Note -->
        <div class="infobox infobox-information">
            <div class="infobox-header">Implementation Status</div>
            <div class="infobox-content">
                <p>
                    <strong>Note:</strong> Full request queue tracking is under development in GoHyphanet.
                </p>
                <p style="margin-top: 10px;">
                    Currently, downloads fetched through the <a href="/">Browse page</a> are processed immediately
                    and do not appear in the queue. Future versions will implement persistent request tracking
                    compatible with the Freenet 0.7 protocol.
                </p>
                <p style="margin-top: 10px;">
                    For now, you can use the FCP interface on port 9481 for programmatic access to the datastore.
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

// serveUploadsPage serves the uploads queue page
// Based on fred-next's QueueToadlet.java (GPL v2+)
func (s *FProxyServer) serveUploadsPage(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Uploads - Freenet</title>
    <style>
        /* Based on Freenet/Hyphanet default theme */

        * {
            margin: 0;
            padding: 0;
        }

        body {
            font-family: Arial, sans-serif;
            font-size: 11pt;
            margin: 0;
            padding: 0;
            background-color: #f5f5f5;
        }

        #topbar {
            background: linear-gradient(to bottom, #667eea 0%, #764ba2 100%);
            border-bottom: 1px solid #ccc;
            min-height: 50px;
            padding: 0.333em;
            color: white;
        }

        #topbar h1 {
            font-size: 1.818em;
            font-weight: normal;
            padding-top: 0.35em;
            text-align: center;
            color: white;
        }

        #navbar {
            float: left;
            position: relative;
            left: 0.667em;
            top: 0.667em;
            width: 11.081em;
            border: 1px solid #ccc;
            background: #fff;
        }

        #navlist {
            list-style-type: none;
            margin: 0;
            padding: 0;
        }

        #navlist li {
            border-bottom: 1px dotted #ddd;
        }

        #navlist a {
            display: block;
            padding: 0.667em;
            text-decoration: none;
            color: #333;
            font-weight: bold;
        }

        #navlist a:hover {
            background-color: #f0f0f0;
        }

        #navlist li.navlist-selected a {
            background-color: #667eea;
            color: white;
        }

        #content {
            margin-top: 0.667em;
            margin-left: 12.5em;
            margin-right: 0.667em;
            position: relative;
        }

        div.infobox {
            background: #fff;
            border: 1px solid #ccc;
            margin-bottom: 0.667em;
            border-radius: 4px;
        }

        div.infobox-header {
            background: #f0f0f0;
            padding: 0.667em;
            border-bottom: 1px dotted #ccc;
            font-weight: bold;
        }

        div.infobox-content {
            padding: 0.667em;
        }

        div.infobox-information {
            border-left: 4px solid #3498db;
        }

        .queue-empty-message {
            padding: 20px;
            text-align: center;
            color: #666;
        }

        .queue-help {
            margin-top: 15px;
            padding: 15px;
            background: #f9f9f9;
            border-radius: 4px;
            border-left: 3px solid #e67e22;
        }

        .queue-help h3 {
            margin-bottom: 10px;
            color: #333;
        }

        .queue-help ul {
            margin-left: 20px;
            margin-top: 10px;
        }

        .queue-help li {
            margin: 5px 0;
        }
    </style>
</head>
<body>
    <div id="topbar">
        <h1>Freenet</h1>
    </div>

    <div id="navbar">
        <ul id="navlist">
            <li><a href="/">Browse</a></li>
            <li><a href="/queue/">Queue</a></li>
            <li class="navlist-selected"><a href="/uploads/">Uploads</a></li>
            <li><a href="/downloads/">Downloads</a></li>
            <li><a href="/config/">Configuration</a></li>
            <li><a href="/plugins/">Plugins</a></li>
            <li><a href="/friends/">Friends</a></li>
            <li><a href="/strangers/">Strangers</a></li>
            <li><a href="/alerts/">Alerts</a></li>
            <li><a href="/statistics/">Statistics</a></li>
        </ul>
    </div>

    <div id="content">
        <div class="infobox infobox-information">
            <div class="infobox-header">Uploads Queue (0/0/0)</div>
            <div class="infobox-content">
                <div class="queue-empty-message">
                    <strong>Global queue is empty</strong>
                    <p style="margin-top: 10px;">There are no upload tasks in the global queue.</p>
                </div>

                <div class="queue-help">
                    <h3>About Uploads</h3>
                    <p>The uploads queue shows all active and completed insert requests to the Freenet network.</p>

                    <h3 style="margin-top: 15px;">How to upload content:</h3>
                    <ul>
                        <li>Use the FCP protocol (port 9481) to insert content programmatically</li>
                        <li>Future versions will include a web-based upload form</li>
                        <li>Uploads can be CHK (content-hash key), SSK (signed-subspace key), or USK (updatable-subspace key)</li>
                    </ul>

                    <h3 style="margin-top: 15px;">Upload types:</h3>
                    <ul>
                        <li><strong>CHK</strong> - Content is hashed and stored anonymously</li>
                        <li><strong>SSK</strong> - Content is signed with your private key</li>
                        <li><strong>USK</strong> - Updatable content that can be versioned</li>
                    </ul>
                </div>
            </div>
        </div>

        <div class="infobox infobox-information">
            <div class="infobox-header">Implementation Status</div>
            <div class="infobox-content">
                <p>
                    <strong>Note:</strong> Upload queue tracking is under development in GoHyphanet.
                </p>
                <p style="margin-top: 10px;">
                    You can currently insert content using the FCP interface on port 9481.
                    Future versions will implement a web-based upload interface and persistent request tracking.
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

// serveDownloadsPage serves the completed downloads page
func (s *FProxyServer) serveDownloadsPage(w http.ResponseWriter, r *http.Request) {
	// For now, redirect to queue page as they're closely related
	http.Redirect(w, r, "/queue/", http.StatusSeeOther)
}
