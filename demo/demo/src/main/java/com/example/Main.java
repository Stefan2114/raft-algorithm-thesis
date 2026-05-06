package com.example;

import com.kvraft.Clerk;
import com.kvraft.GetResult;
import java.util.Arrays;
import java.util.List;

public class Main {
    public static void main(String[] args) {
        // Ports matching cluster.json (8000-8004)
        List<String> addresses = Arrays.asList(
            "localhost:8000", 
            "localhost:8001", 
            "localhost:8002", 
            "localhost:8003", 
            "localhost:8004"
        );

        System.out.println("--- KVRaft Java Client Demo ---");

        try (Clerk clerk = new Clerk(addresses)) {
            // 1. Put a value
            System.out.println("Putting [key=greet, value=hello-from-java]...");
            clerk.put("greet", "hello-stef", 1).join();
            System.out.println("Put successful!");

            // 2. Get the value back
            System.out.println("Getting [key=greet]...");
            GetResult result = clerk.get("greet").join();
            System.out.println("Result: " + result.getValue() + " (Version: " + result.getVersion() + ")");

        } catch (Exception e) {
            System.err.println("Error: " + e.getMessage());
            e.printStackTrace();
        }
    }
}
