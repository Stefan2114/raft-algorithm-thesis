package com.kvraft;

import java.util.List;
import java.util.concurrent.CompletableFuture;

public class Main {
    public static void main(String[] args) {
        // Define the cluster node addresses (matching config/cluster.json)
        List<String> clusterAddresses = List.of(
                "localhost:8000",
                "localhost:8001",
                "localhost:8002"
        );

        System.out.println("--- Starting KVRaft Java Clerk Demo (Best Practices Version) ---");

        try (Clerk clerk = new Clerk(clusterAddresses)) {
            
            // 1. Synchronous usage
            System.out.println("\n[Action] Performing Synchronous Put...");
            clerk.put("hello", "java-best-practices", 0).join();
            System.out.println("[Success] Put operation finished.");

            System.out.println("\n[Action] Performing Synchronous Get...");
            GetResult getResult = clerk.get("hello").join();
            System.out.println("[Result] Found value: " + getResult.getValue() + " (Version: " + getResult.getVersion() + ")");

            // 2. Asynchronous usage
            System.out.println("\n[Action] Performing Asynchronous Put...");
            CompletableFuture<Void> asyncPut = clerk.put("async-key", "async-best-practices", 0)
                    .thenAccept(v -> System.out.println("[Callback] Async Put successful!"))
                    .exceptionally(ex -> {
                        System.err.println("[Error] Async Put failed: " + ex.getMessage());
                        return null;
                    });

            asyncPut.join();

            System.out.println("\n--- Demo Completed Successfully ---");

        } catch (Exception e) {
            System.err.println("Unexpected Error: " + e.getMessage());
            e.printStackTrace();
        }
    }
}
