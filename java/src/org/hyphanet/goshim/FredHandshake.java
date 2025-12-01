package org.hyphanet.goshim;

import java.io.*;
import java.net.*;
import java.nio.ByteBuffer;
import java.security.*;
import java.security.spec.*;
import javax.crypto.*;
import java.util.*;

/**
 * Simplified handshake implementation that mimics Fred's JFK protocol.
 * This is a standalone version that doesn't require the full Fred codebase.
 */
public class FredHandshake {

    private static final int IDENTITY_LENGTH = 32;
    private static final int HASH_LENGTH = 32;
    private static final int NONCE_SIZE = 32;

    private final KeyPair ecdsaKeyPair;
    private final byte[] identity;
    private final byte[] identityHash;
    private final SecureRandom random;

    public FredHandshake() throws Exception {
        this.random = new SecureRandom();

        // Generate ECDSA P-256 keypair
        KeyPairGenerator keyGen = KeyPairGenerator.getInstance("EC");
        ECGenParameterSpec ecSpec = new ECGenParameterSpec("secp256r1");
        keyGen.initialize(ecSpec, random);
        this.ecdsaKeyPair = keyGen.generateKeyPair();

        // Generate identity from public key
        this.identity = ecdsaKeyPair.getPublic().getEncoded();

        // Compute identity hash (setup key)
        MessageDigest sha256 = MessageDigest.getInstance("SHA-256");
        this.identityHash = sha256.digest(identity);

        System.err.println("[HANDSHAKE] Initialized with identity hash: " +
            bytesToHex(identityHash).substring(0, 16));
    }

    public HandshakeResult connectToSeedNode(String host, int port) throws Exception {
        return connectToSeedNode(host, port, null);
    }

    public HandshakeResult connectToSeedNode(String host, int port, String seedIdentityBase64) throws Exception {
        System.err.println("[HANDSHAKE] Connecting to seed node " + host + ":" + port);

        // If seed identity is provided, use it for encryption
        byte[] encryptionKey = identityHash; // Default to our own
        if (seedIdentityBase64 != null && !seedIdentityBase64.isEmpty()) {
            try {
                // Decode base64 identity (Freenet uses modified base64 with ~ instead of =)
                String normalizedBase64 = seedIdentityBase64.replace('~', '=');

                // Add padding if needed
                int padding = (4 - (normalizedBase64.length() % 4)) % 4;
                normalizedBase64 = normalizedBase64 + "=".repeat(padding);

                byte[] seedIdentity = java.util.Base64.getDecoder().decode(normalizedBase64);

                // Hash it to get the encryption key
                MessageDigest sha256 = MessageDigest.getInstance("SHA-256");
                encryptionKey = sha256.digest(seedIdentity);

                System.err.println("[HANDSHAKE] Using seed node's identity hash for encryption: " +
                    bytesToHex(encryptionKey).substring(0, 16));
            } catch (Exception e) {
                System.err.println("[HANDSHAKE] Warning: Failed to decode seed identity, using own: " + e.getMessage());
            }
        }

        final byte[] finalEncryptionKey = encryptionKey;

        InetAddress addr = InetAddress.getByName(host);
        DatagramSocket socket = new DatagramSocket();
        socket.setSoTimeout(10000); // 10 second timeout

        try {
            // Generate ECDH keypair for this handshake
            KeyPairGenerator ecdhGen = KeyPairGenerator.getInstance("EC");
            ECGenParameterSpec ecSpec = new ECGenParameterSpec("secp256r1");
            ecdhGen.initialize(ecSpec, random);
            KeyPair ecdhKeyPair = ecdhGen.generateKeyPair();

            // Generate nonce
            byte[] nonce = new byte[NONCE_SIZE];
            random.nextBytes(nonce);

            // Build JFK Message 1
            byte[] message1 = buildJFKMessage1(nonce, ecdhKeyPair.getPublic());

            System.err.println("[HANDSHAKE] Sending JFK Message 1 (" + message1.length + " bytes)");

            // Send packet (use finalEncryptionKey for anonymous initiator)
            byte[] packet = wrapInAuthPacketWithKey(message1, 1, 10, 0, 1, finalEncryptionKey); // version=1, negType=10, phase=0, setupType=1
            DatagramPacket sendPacket = new DatagramPacket(packet, packet.length, addr, port);
            socket.send(sendPacket);

            System.err.println("[HANDSHAKE] Packet sent, waiting for response...");

            // Wait for response
            byte[] recvBuffer = new byte[4096];
            DatagramPacket recvPacket = new DatagramPacket(recvBuffer, recvBuffer.length);

            try {
                socket.receive(recvPacket);
                System.err.println("[HANDSHAKE] Received response! (" + recvPacket.getLength() + " bytes)");

                // Extract response data
                byte[] responsePacket = Arrays.copyOf(recvPacket.getData(), recvPacket.getLength());

                try {
                    // Try to decrypt the response using the same key we used for encryption
                    byte[] decryptedPayload = PacketCrypto.decryptAuthPacket(responsePacket, finalEncryptionKey);

                    System.err.println("[HANDSHAKE] Successfully decrypted response!");
                    System.err.println("[HANDSHAKE] Decrypted payload: " + decryptedPayload.length + " bytes");

                    // Check if this is JFK Message 2
                    if (decryptedPayload.length >= 4) {
                        int version = decryptedPayload[0] & 0xFF;
                        int negType = decryptedPayload[1] & 0xFF;
                        int phase = decryptedPayload[2] & 0xFF;

                        System.err.println("[HANDSHAKE] Version: " + version + ", NegType: " + negType + ", Phase: " + phase);

                        if (phase == 1) {
                            System.err.println("[HANDSHAKE] ✓ Received JFK Message 2!");

                            HandshakeResult result = new HandshakeResult();
                            result.success = true;
                            result.message = "Successfully received and decrypted JFK Message 2 from seed node!";
                            result.responseLength = recvPacket.getLength();
                            result.remoteAddress = recvPacket.getAddress().getHostAddress();
                            result.remotePort = recvPacket.getPort();

                            return result;
                        }
                    }

                    HandshakeResult result = new HandshakeResult();
                    result.success = true;
                    result.message = "Received and decrypted response (unexpected phase)";
                    result.responseLength = recvPacket.getLength();
                    result.remoteAddress = recvPacket.getAddress().getHostAddress();
                    result.remotePort = recvPacket.getPort();

                    return result;

                } catch (Exception e) {
                    System.err.println("[HANDSHAKE] Failed to decrypt response: " + e.getMessage());

                    HandshakeResult result = new HandshakeResult();
                    result.success = false;
                    result.message = "Received response but failed to decrypt: " + e.getMessage();

                    return result;
                }

            } catch (SocketTimeoutException e) {
                System.err.println("[HANDSHAKE] No response received (timeout)");

                HandshakeResult result = new HandshakeResult();
                result.success = false;
                result.message = "No response from seed node (this is expected with simplified packet format)";

                return result;
            }

        } finally {
            socket.close();
        }
    }

    private byte[] buildJFKMessage1(byte[] nonce, PublicKey ecdhPublicKey) throws Exception {
        // Hash the nonce
        MessageDigest sha256 = MessageDigest.getInstance("SHA-256");
        byte[] nonceHash = sha256.digest(nonce);

        // Get public key bytes (uncompressed format for P-256 = 65 bytes)
        byte[] publicKeyBytes = getUncompressedPublicKey(ecdhPublicKey);

        // Message format: nonceHash(32) + publicKey(65) + identityHash(32) = 129 bytes
        ByteArrayOutputStream baos = new ByteArrayOutputStream();
        baos.write(nonceHash);
        baos.write(publicKeyBytes);
        baos.write(identityHash); // For unknown initiator (seed node)

        return baos.toByteArray();
    }

    private byte[] getUncompressedPublicKey(PublicKey publicKey) throws Exception {
        // Get the EC public key point
        KeyFactory keyFactory = KeyFactory.getInstance("EC");
        ECPublicKeySpec ecSpec = keyFactory.getKeySpec(publicKey, ECPublicKeySpec.class);
        java.security.spec.ECPoint point = ecSpec.getW();

        // Convert to uncompressed format: 0x04 + X + Y
        byte[] x = point.getAffineX().toByteArray();
        byte[] y = point.getAffineY().toByteArray();

        // Ensure 32 bytes each (remove sign byte if present)
        x = ensureLength(x, 32);
        y = ensureLength(y, 32);

        byte[] result = new byte[65];
        result[0] = 0x04; // Uncompressed point marker
        System.arraycopy(x, 0, result, 1, 32);
        System.arraycopy(y, 0, result, 33, 32);

        return result;
    }

    private byte[] ensureLength(byte[] data, int length) {
        if (data.length == length) {
            return data;
        } else if (data.length > length) {
            // Remove leading zeros/sign byte
            return Arrays.copyOfRange(data, data.length - length, data.length);
        } else {
            // Pad with leading zeros
            byte[] padded = new byte[length];
            System.arraycopy(data, 0, padded, length - data.length, data.length);
            return padded;
        }
    }

    private byte[] wrapInAuthPacketWithKey(byte[] payload, int version, int negType, int phase, int setupType, byte[] key) throws Exception {
        // Build packet header: version + negType + phase + setupType + payload
        byte[] fullPayload = new byte[4 + payload.length];
        fullPayload[0] = (byte) version;
        fullPayload[1] = (byte) negType;
        fullPayload[2] = (byte) phase;
        fullPayload[3] = (byte) setupType;
        System.arraycopy(payload, 0, fullPayload, 4, payload.length);

        // For anonymous initiator (seed node), use cipher based on SEED's identity hash
        // Create authenticated encrypted packet using PCFB mode
        byte[] packet = PacketCrypto.createAuthPacket(fullPayload, key, random);

        System.err.println("[HANDSHAKE] Created encrypted auth packet: " + packet.length + " bytes");
        return packet;
    }

    private static String bytesToHex(byte[] bytes) {
        StringBuilder sb = new StringBuilder();
        for (byte b : bytes) {
            sb.append(String.format("%02x", b));
        }
        return sb.toString();
    }

    public static class HandshakeResult {
        public boolean success;
        public String message;
        public int responseLength;
        public String remoteAddress;
        public int remotePort;
        public byte[] sessionKey;
    }
}
