package com.example;

import com.kvraft.Clerk;
import com.kvraft.GetResult;
import com.kvraft.KVRaftException;
import com.kvraft.kv.v1.Status;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Paths;
import java.nio.file.StandardOpenOption;
import java.util.Arrays;
import java.util.List;

public class Main {
    private static final String LOCK_KEY = "lock:shared_file";
    private static final long LEASE_DURATION_MS = 6000; 
    private static final long ACQUIRE_RETRY_INTERVAL_MS = 200; 

    public static void main(String[] args) {
        String instanceId = System.getenv("INSTANCE_ID");
        if (instanceId == null || instanceId.isEmpty()) {
            instanceId = "client-local-" + System.currentTimeMillis();
        }

        String inputPath = System.getenv("INPUT_PATH");
        if (inputPath == null || inputPath.isEmpty()) {
            inputPath = "/shared/input.txt";
        }

        String outputPath = System.getenv("OUTPUT_PATH");
        if (outputPath == null || outputPath.isEmpty()) {
            outputPath = "/shared/output.txt";
        }

        System.out.printf("[%s] Starting instance... Connecting to KVRaft cluster.%n", instanceId);

        List<String> addresses = Arrays.asList(
            "127.0.0.1:8000",
            "127.0.0.1:8001",
            "127.0.0.1:8002",
            "127.0.0.1:8003",
            "127.0.0.1:8004"
        );

        try (Clerk clerk = new Clerk(addresses)) {
            acquireLockAndExecute(clerk, instanceId, inputPath, outputPath);
        } catch (Exception e) {
            System.err.printf("[%s] Unhandled exception: %s%n", instanceId, e.getMessage());
            e.printStackTrace();
            System.exit(1);
        }
    }

    private static void acquireLockAndExecute(Clerk clerk, String instanceId, String inputPath, String outputPath) {
        boolean gotLock = false;

        while (!gotLock) {
            try {
                System.out.printf("[%s] Reading lock key '%s'...%n", instanceId, LOCK_KEY);
                GetResult getRes = null;
                boolean noKey = false;
                try {
                    getRes = clerk.get(LOCK_KEY).join();
                } catch (Exception e) {
                    Throwable cause = e.getCause();
                    if (cause instanceof KVRaftException && ((KVRaftException) cause).getStatus() == Status.ERR_NO_KEY) {
                        noKey = true;
                    } else {
                        throw e; 
                    }
                }

                long currentTime = System.currentTimeMillis();
                long expireTime = currentTime + LEASE_DURATION_MS;
                String lockValue = instanceId + ":" + expireTime;

                if (noKey) {
                    System.out.printf("[%s] Lock key is empty. Trying to acquire lock (version 0)...%n", instanceId);
                    clerk.put(LOCK_KEY, lockValue, 0).join();
                    System.out.printf("[%s] ---> LOCK ACQUIRED! (as version 1)%n", instanceId);
                    gotLock = true;
                } else {
                    String val = getRes.getValue();
                    long version = getRes.getVersion();
                    String[] parts = val.split(":");
                    String owner = parts[0];
                    long exp = parts.length > 1 ? Long.parseLong(parts[1]) : 0;

                    if (owner.equals("unlocked") || currentTime > exp) {
                        if (currentTime > exp && !owner.equals("unlocked")) {
                            System.out.printf("[%s] Lock expired (held by %s, expired at %d, current %d). Trying to steal lock...%n", 
                                instanceId, owner, exp, currentTime);
                        } else {
                            System.out.printf("[%s] Lock is unlocked. Trying to acquire lock (version %d)...%n", instanceId, version);
                        }

                        clerk.put(LOCK_KEY, lockValue, version).join();
                        System.out.printf("[%s] ---> LOCK ACQUIRED! (version %d)%n", instanceId, version + 1);
                        gotLock = true;
                    } else {
                        System.out.printf("[%s] Lock is busy (held by %s, expires in %d ms). Waiting...%n", 
                            instanceId, owner, exp - currentTime);
                        Thread.sleep(ACQUIRE_RETRY_INTERVAL_MS);
                    }
                }
            } catch (Exception e) {
                System.err.printf("[%s] Lock acquisition attempt failed: %s. Retrying...%n", instanceId, e.getMessage());
                try {
                    Thread.sleep(ACQUIRE_RETRY_INTERVAL_MS);
                } catch (InterruptedException ex) {
                    Thread.currentThread().interrupt();
                    return;
                }
            }
        }

        try {
            System.out.printf("[%s] Performing operations under lock protection...%n", instanceId);
            
            List<String> lines = Files.readAllLines(Paths.get(inputPath));
            String content = String.join("\n", lines) + "\n";
            
            String formattedOutput = String.format("--- Start of Write by %s ---%n%s--- End of Write by %s ---%n", 
                instanceId, content, instanceId);

            Files.write(Paths.get(outputPath), formattedOutput.getBytes(), 
                StandardOpenOption.CREATE, StandardOpenOption.APPEND);

            System.out.printf("[%s] Appended content from %s to %s.%n", instanceId, inputPath, outputPath);

            Thread.sleep(1000);

        } catch (IOException e) {
            System.err.printf("[%s] File I/O Error: %s%n", instanceId, e.getMessage());
        } catch (InterruptedException e) {
            System.err.printf("[%s] Interrupted: %s%n", instanceId, e.getMessage());
            Thread.currentThread().interrupt();
        }

        try {
            System.out.printf("[%s] Releasing lock...%n", instanceId);
            GetResult getRes = clerk.get(LOCK_KEY).join();
            String[] parts = getRes.getValue().split(":");
            String owner = parts[0];
            if (owner.equals(instanceId)) {
                clerk.put(LOCK_KEY, "unlocked:0", getRes.getVersion()).join();
                System.out.printf("[%s] <--- LOCK RELEASED successfully!%n", instanceId);
            } else {
                System.out.printf("[%s] Lock lease already expired or stolen by %s. Skipping release.%n", instanceId, owner);
            }
        } catch (Exception e) {
            System.err.printf("[%s] Lock release failed: %s (Lease will expire naturally)%n", instanceId, e.getMessage());
        }
    }
}
