package org.hyphanet.goshim;

import java.io.*;
import java.net.*;
import java.security.*;
import java.security.spec.*;
import javax.crypto.*;
import javax.crypto.spec.*;
import java.util.*;
import com.google.gson.*;

/**
 * Java shim that performs Hyphanet JFK handshakes and returns session keys to Go.
 * Communicates via JSON over stdin/stdout.
 */
public class HandshakeShim {

    private static final Gson gson = new Gson();

    public static void main(String[] args) {
        try {
            System.err.println("[SHIM] Starting Hyphanet Handshake Shim");

            BufferedReader reader = new BufferedReader(new InputStreamReader(System.in));
            PrintWriter writer = new PrintWriter(System.out, true);

            while (true) {
                String line = reader.readLine();
                if (line == null || line.equals("EXIT")) {
                    break;
                }

                try {
                    Request request = gson.fromJson(line, Request.class);
                    Response response = handleRequest(request);
                    writer.println(gson.toJson(response));
                } catch (Exception e) {
                    Response error = new Response();
                    error.success = false;
                    error.error = e.getMessage();
                    writer.println(gson.toJson(error));
                    System.err.println("[SHIM] Error: " + e.getMessage());
                    e.printStackTrace(System.err);
                }
            }

        } catch (Exception e) {
            System.err.println("[SHIM] Fatal error: " + e.getMessage());
            e.printStackTrace(System.err);
            System.exit(1);
        }
    }

    private static Response handleRequest(Request req) throws Exception {
        System.err.println("[SHIM] Handling request: " + req.command);

        switch (req.command) {
            case "HANDSHAKE":
                return performHandshake(req);
            case "PING":
                return ping();
            default:
                throw new IllegalArgumentException("Unknown command: " + req.command);
        }
    }

    private static Response ping() {
        Response resp = new Response();
        resp.success = true;
        resp.data = Map.of("status", "alive");
        return resp;
    }

    private static Response performHandshake(Request req) throws Exception {
        String host = (String) req.params.get("host");
        int port = ((Double) req.params.get("port")).intValue();
        String seedIdentity = (String) req.params.get("seedIdentity"); // Optional

        System.err.println("[SHIM] Attempting handshake with " + host + ":" + port);
        if (seedIdentity != null) {
            System.err.println("[SHIM] Using seed identity: " + seedIdentity.substring(0, 16) + "...");
        }

        FredHandshake handshake = new FredHandshake();
        FredHandshake.HandshakeResult result = handshake.connectToSeedNode(host, port, seedIdentity);

        Response resp = new Response();
        resp.success = result.success;

        if (result.success) {
            resp.data = Map.of(
                "message", result.message,
                "responseLength", result.responseLength,
                "remoteAddress", result.remoteAddress,
                "remotePort", result.remotePort
            );
        } else {
            resp.error = result.message;
            resp.data = Map.of(
                "host", host,
                "port", port
            );
        }

        return resp;
    }

    static class Request {
        String command;
        Map<String, Object> params;
    }

    static class Response {
        boolean success;
        String error;
        Map<String, Object> data;
    }
}
