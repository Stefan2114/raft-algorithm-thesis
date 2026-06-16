package com.example;

import com.kvraft.Clerk;
import com.kvraft.GetResult;
import com.kvraft.KVRaftException;
import com.kvraft.kv.v1.Status;

import java.util.Arrays;
import java.util.List;
import java.util.Random;

public class LoadGenerator {
    public static void main(String[] args) {
        System.out.println("--- Starting KVRaft Cluster Load Generator ---");
        System.out.println("Generating continuous traffic to populate Grafana dashboards...");

        List<String> addresses = Arrays.asList(
            "127.0.0.1:8000",
            "127.0.0.1:8001",
            "127.0.0.1:8002",
            "127.0.0.1:8003",
            "127.0.0.1:8004"
        );

        Random rand = new Random();
        long opsCount = 0;

        try (Clerk clerk = new Clerk(addresses)) {
            while (true) {
                String key = "load_key_" + rand.nextInt(10);
                String value = "val_" + rand.nextInt(1000);

                try {
                    GetResult getRes = null;
                    boolean noKey = false;
                    try {
                        getRes = clerk.get(key).join();
                    } catch (Exception e) {
                        Throwable cause = e.getCause();
                        if (cause instanceof KVRaftException && ((KVRaftException) cause).getStatus() == Status.ERR_NO_KEY) {
                            noKey = true;
                        } else {
                            throw e;
                        }
                    }

                    long version = noKey ? 0 : getRes.getVersion();
                    clerk.put(key, value, version).join();
                    
                    opsCount += 2;

                } catch (Exception e) {
                    System.err.printf("Operation failed (expected under chaos): %s%n", e.getMessage());
                }

                if (opsCount > 0 && opsCount % 50 == 0) {
                    System.out.printf("[%d] Sent %d load operations successfully.%n", System.currentTimeMillis(), opsCount);
                }

                Thread.sleep(100 + rand.nextInt(150));
            }
        } catch (InterruptedException e) {
            System.out.println("Load Generator stopped by interrupt.");
        } catch (Exception e) {
            System.err.println("Load Generator terminated with error: " + e.getMessage());
            e.printStackTrace();
        }
    }
}
