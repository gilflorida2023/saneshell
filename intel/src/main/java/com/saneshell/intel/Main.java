package com.saneshell.intel;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.*;
import java.net.*;
import java.nio.channels.*;
import java.nio.charset.StandardCharsets;
import java.nio.file.*;
import java.util.concurrent.atomic.AtomicBoolean;

public class Main {

    private static final Logger log = LoggerFactory.getLogger(Main.class);
    private static final ObjectMapper json = new ObjectMapper();
    private static final AtomicBoolean running = new AtomicBoolean(true);

    public static void main(String[] args) throws Exception {
        int uid = getUid();
        Path socketPath = Paths.get("/tmp", "saneshell-" + uid + ".sock");

        // Remove stale socket
        Files.deleteIfExists(socketPath);

        UnixDomainSocketAddress addr = UnixDomainSocketAddress.of(socketPath);

        log.info("saneshell-intel v{} starting", BuildConfig.VERSION);
        log.info("protocol v{}", BuildConfig.PROTOCOL_VERSION);
        log.info("socket: {}", socketPath);

        Runtime.getRuntime().addShutdownHook(new Thread(() -> {
            running.set(false);
            try { Files.deleteIfExists(socketPath); } catch (Exception ignored) {}
            log.info("shutdown complete");
        }));

        try (ServerSocketChannel server = ServerSocketChannel.open(StandardProtocolFamily.UNIX)) {
            server.bind(addr);

            log.info("listening...");

            while (running.get()) {
                try (SocketChannel client = server.accept()) {
                    handleClient(client);
                } catch (IOException e) {
                    if (running.get()) {
                        log.error("accept error: {}", e.getMessage());
                    }
                }
            }
        }

        Files.deleteIfExists(socketPath);
    }

    private static void handleClient(SocketChannel client) {
        try {
            BufferedReader reader = new BufferedReader(
                Channels.newReader(client, StandardCharsets.UTF_8));
            BufferedWriter writer = new BufferedWriter(
                Channels.newWriter(client, StandardCharsets.UTF_8));

            String line;
            while ((line = reader.readLine()) != null) {
                if (line.trim().isEmpty()) continue;

                switch (getMessageType(line)) {
                    case "complete":
                        handleComplete(line, writer);
                        break;
                    case "preview":
                        handlePreview(line, writer);
                        break;
                    case "learn":
                        handleLearn(line, writer);
                        break;
                    case "suggest":
                        handleSuggest(line, writer);
                        break;
                    default:
                        // Unknown message type — send ack
                        writer.write("{\"type\":\"ack\"}\n");
                        writer.flush();
                }
            }
        } catch (IOException e) {
            log.debug("client disconnected: {}", e.getMessage());
        }
    }

    private static String getMessageType(String line) {
        try {
            var root = json.readTree(line);
            var type = root.get("type");
            return type != null ? type.asText("") : "";
        } catch (Exception e) {
            return "";
        }
    }

    private static void handleComplete(String request, BufferedWriter writer) throws IOException {
        log.debug("completion request received");
        writer.write("{\"type\":\"completions\",\"items\":[]}\n");
        writer.flush();
    }

    private static void handlePreview(String request, BufferedWriter writer) throws IOException {
        log.debug("preview request received");
        writer.write("{\"type\":\"preview\",\"risk\":\"none\",\"warnings\":[]}\n");
        writer.flush();
    }

    private static void handleLearn(String request, BufferedWriter writer) throws IOException {
        log.debug("learn event received");
        writer.write("{\"type\":\"ack\"}\n");
        writer.flush();
    }

    private static void handleSuggest(String request, BufferedWriter writer) throws IOException {
        log.debug("suggest request received");
        writer.write("{\"type\":\"suggestions\",\"items\":[]}\n");
        writer.flush();
    }

    private static int getUid() {
        try {
            var uid = System.getProperty("uid");
            if (uid != null) return Integer.parseInt(uid);
        } catch (Exception ignored) {}
        return 1000; // fallback
    }
}
