package org.hyphanet.goshim;

import javax.crypto.Cipher;
import javax.crypto.spec.SecretKeySpec;
import java.security.*;
import java.util.Arrays;

/**
 * Implements FNP packet encryption/decryption (PCFB mode with Rijndael/AES-256)
 * Based on Fred's PCFBMode and FNPPacketMangler
 */
public class PacketCrypto {

    /**
     * PCFB (Per-block Cipher Feedback) Mode implementation
     * This is what Freenet uses for packet encryption
     */
    public static class PCFBMode {
        private final Cipher cipher;
        private byte[] registerBuf;
        private int registerPointer;

        public PCFBMode(byte[] key, byte[] iv) throws Exception {
            // Use AES-256 (Rijndael with 256-bit key)
            SecretKeySpec keySpec = new SecretKeySpec(Arrays.copyOf(key, 32), "AES");
            cipher = Cipher.getInstance("AES/ECB/NoPadding");
            cipher.init(Cipher.ENCRYPT_MODE, keySpec);

            // Initialize register with IV
            this.registerBuf = Arrays.copyOf(iv, 32);
            this.registerPointer = 0;
        }

        /**
         * Encrypt a single byte
         */
        public byte encipher(byte b) {
            if (registerPointer == 0) {
                // Encrypt the register
                try {
                    registerBuf = cipher.doFinal(registerBuf);
                } catch (Exception e) {
                    throw new RuntimeException(e);
                }
            }

            byte result = (byte) (b ^ registerBuf[registerPointer]);
            registerBuf[registerPointer] = result;
            registerPointer = (registerPointer + 1) % 32;

            return result;
        }

        /**
         * Encrypt a block of data
         */
        public void blockEncipher(byte[] data, int offset, int length) {
            for (int i = 0; i < length; i++) {
                data[offset + i] = encipher(data[offset + i]);
            }
        }

        /**
         * Decrypt a single byte (same as encipher in PCFB)
         */
        public byte decipher(byte b) {
            if (registerPointer == 0) {
                try {
                    registerBuf = cipher.doFinal(registerBuf);
                } catch (Exception e) {
                    throw new RuntimeException(e);
                }
            }

            byte result = (byte) (b ^ registerBuf[registerPointer]);
            registerBuf[registerPointer] = b;
            registerPointer = (registerPointer + 1) % 32;

            return result;
        }

        /**
         * Decrypt a block of data
         */
        public void blockDecipher(byte[] data, int offset, int length) {
            for (int i = 0; i < length; i++) {
                data[offset + i] = decipher(data[offset + i]);
            }
        }

        public static int lengthIV() {
            return 32; // AES-256 block size
        }
    }

    /**
     * Create an authenticated encrypted packet for JFK handshake
     * Format: IV + encrypted(hash) + encrypted(length) + encrypted(payload) + padding
     */
    public static byte[] createAuthPacket(byte[] payload, byte[] key, SecureRandom random) throws Exception {
        // Generate random IV (32 bytes for AES-256)
        byte[] iv = new byte[32];
        random.nextBytes(iv);

        // Hash the payload
        MessageDigest sha256 = MessageDigest.getInstance("SHA-256");
        byte[] hash = sha256.digest(payload);

        // Create PCFB mode
        PCFBMode pcfb = new PCFBMode(key, iv);

        // Encrypt hash
        byte[] encryptedHash = Arrays.copyOf(hash, hash.length);
        pcfb.blockEncipher(encryptedHash, 0, encryptedHash.length);

        // Encrypt length (2 bytes)
        int payloadLength = payload.length;
        byte lengthByte1 = pcfb.encipher((byte) (payloadLength >> 8));
        byte lengthByte2 = pcfb.encipher((byte) payloadLength);

        // Encrypt payload
        byte[] encryptedPayload = Arrays.copyOf(payload, payload.length);
        pcfb.blockEncipher(encryptedPayload, 0, encryptedPayload.length);

        // Add random padding (0-100 bytes)
        int paddingLength = random.nextInt(100);
        byte[] padding = new byte[paddingLength];
        random.nextBytes(padding);

        // Assemble packet: IV + encryptedHash + encryptedLength + encryptedPayload + padding
        byte[] packet = new byte[iv.length + encryptedHash.length + 2 + encryptedPayload.length + paddingLength];
        int offset = 0;

        System.arraycopy(iv, 0, packet, offset, iv.length);
        offset += iv.length;

        System.arraycopy(encryptedHash, 0, packet, offset, encryptedHash.length);
        offset += encryptedHash.length;

        packet[offset++] = lengthByte1;
        packet[offset++] = lengthByte2;

        System.arraycopy(encryptedPayload, 0, packet, offset, encryptedPayload.length);
        offset += encryptedPayload.length;

        System.arraycopy(padding, 0, packet, offset, paddingLength);

        return packet;
    }

    /**
     * Decrypt an authenticated packet
     */
    public static byte[] decryptAuthPacket(byte[] packet, byte[] key) throws Exception {
        if (packet.length < 66) { // IV(32) + hash(32) + length(2) minimum
            throw new IllegalArgumentException("Packet too short");
        }

        // Extract IV
        byte[] iv = Arrays.copyOfRange(packet, 0, 32);

        // Create PCFB mode
        PCFBMode pcfb = new PCFBMode(key, iv);

        // Decrypt hash
        byte[] encryptedHash = Arrays.copyOfRange(packet, 32, 64);
        byte[] hash = Arrays.copyOf(encryptedHash, encryptedHash.length);
        pcfb.blockDecipher(hash, 0, hash.length);

        // Decrypt length
        int length = ((pcfb.decipher(packet[64]) & 0xFF) << 8) | (pcfb.decipher(packet[65]) & 0xFF);

        if (66 + length > packet.length) {
            throw new IllegalArgumentException("Invalid length in packet");
        }

        // Decrypt payload
        byte[] encryptedPayload = Arrays.copyOfRange(packet, 66, 66 + length);
        byte[] payload = Arrays.copyOf(encryptedPayload, encryptedPayload.length);
        pcfb.blockDecipher(payload, 0, payload.length);

        // Verify hash
        MessageDigest sha256 = MessageDigest.getInstance("SHA-256");
        byte[] computedHash = sha256.digest(payload);

        if (!Arrays.equals(hash, computedHash)) {
            throw new SecurityException("Hash mismatch - packet corrupted or wrong key");
        }

        return payload;
    }
}
