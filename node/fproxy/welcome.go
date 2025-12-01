package fproxy

import (
	"fmt"
	"net/http"
)

// serveWelcomePage serves the Hyphanet welcome/home page
// This is based on the WelcomeToadlet from fred-next (GPL v2+)
// Ported from: freenet/clients/http/WelcomeToadlet.java and PageMaker.java
func (s *FProxyServer) serveWelcomePage(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Freenet</title>
    <style>
        /* Based on Freenet/Hyphanet default theme - baseelements.css */

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
            background-image: url(/static/logo.png);
            background-position: 30px 3px;
            background-repeat: no-repeat;
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

        /* Forms and Inputs */
        #keyfetchbox input[type="text"] {
            width: 90%;
            padding: 8px;
            border: 1px solid #ccc;
            border-radius: 3px;
            font-size: 14px;
        }

        input[type="submit"] {
            padding: 8px 20px;
            background: #3498db;
            color: white;
            border: none;
            border-radius: 3px;
            cursor: pointer;
            font-size: 14px;
        }

        input[type="submit"]:hover {
            background: #2980b9;
        }

        .fetch-key-label {
            font-weight: bold;
            display: block;
            margin-bottom: 5px;
        }

        /* Bookmarks */
        ul#bookmarks {
            list-style-type: none;
            padding-left: 0;
        }

        ul#bookmarks li {
            margin: 8px 0;
        }

        .bookmarks-header-text {
            color: #333;
            text-decoration: none;
            font-size: 14px;
        }

        .edit-bracket {
            color: #666;
            margin: 0 2px;
        }

        .interfacelink {
            color: #3498db;
            text-decoration: none;
        }

        .interfacelink:hover {
            text-decoration: underline;
        }

        /* Version Info */
        .freenet-full-version,
        .freenet-ext-version {
            display: block;
            margin: 5px 0;
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
            <li class="navlist-selected"><a href="/" title="Browse Freenet">Browse</a></li>
            <li><a href="/queue/" title="View downloads queue">Queue</a></li>
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
        <!-- Fetch Key Box -->
        <div class="infobox infobox-normal" id="keyfetchbox">
            <div class="infobox-header">Browse Freenet</div>
            <div class="infobox-content">
                <form method="POST" action="/">
                    <div>
                        <span class="fetch-key-label">Freenet URI:</span>
                        <input type="text" name="key" size="80" placeholder="CHK@..., SSK@..., KSK@..., or USK@...">
                        <input type="submit" value="Fetch">
                    </div>
                </form>
                <p style="margin-top: 10px; font-size: 13px; color: #666;">
                    Enter a Freenet URI to fetch content from the network.
                    Freenet URIs look like: <code>CHK@...</code>, <code>SSK@...</code>, <code>KSK@...</code>, or <code>USK@...</code>
                </p>
            </div>
        </div>

        <!-- Bookmarks -->
        <div class="infobox infobox-normal bookmarks-box">
            <div class="infobox-header">
                <a class="bookmarks-header-text" title="Your personal Freenet bookmarks">My Bookmarks</a>
                <span class="edit-bracket">[</span><a href="/bookmarkEditor/" class="interfacelink">edit</a><span class="edit-bracket">]</span>
            </div>
            <div class="infobox-content">
                <ul id="bookmarks">
                    <li>No bookmarks configured. Add bookmarks to this list <a href="/bookmarkEditor/" class="interfacelink">here</a>.</li>
                </ul>
            </div>
        </div>

        <!-- Version Information -->
        <div class="infobox infobox-information">
            <div class="infobox-header">Freenet Version</div>
            <div class="infobox-content">
                <span class="freenet-full-version">
                    <strong>GoHyphanet:</strong> Version 0.1.0 (Build 1)
                </span>
                <span class="freenet-ext-version">
                    <strong>Protocol:</strong> Freenet 0.7 compatible
                </span>
                <br>
                <span>
                    <strong>Ports:</strong> FProxy: 8888, FCP: 9481
                </span>
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
