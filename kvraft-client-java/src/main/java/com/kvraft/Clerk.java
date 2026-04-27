package com.kvraft;

import com.google.common.util.concurrent.FutureCallback;
import com.google.common.util.concurrent.Futures;
import com.google.common.util.concurrent.ListenableFuture;
import com.google.common.util.concurrent.MoreExecutors;
import com.kvraft.kv.v1.*;
import io.grpc.ManagedChannel;
import io.grpc.ManagedChannelBuilder;

import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.function.Function;

import javax.annotation.Nonnull;

/**
 * An industry-standard, asynchronous gRPC client for the KVRaft cluster.
 */
public class Clerk implements AutoCloseable {
    private final List<ManagedChannel> channels;
    private final List<KVGrpc.KVFutureStub> stubs;
    private final AtomicInteger lastLeader = new AtomicInteger(0);
    private final int totalNodes;

    public Clerk(List<String> addresses) {
        this.totalNodes = addresses.size();
        this.channels = new ArrayList<>(totalNodes);
        this.stubs = new ArrayList<>(totalNodes);

        for (String addr : addresses) {
            ManagedChannel channel = ManagedChannelBuilder.forTarget(addr)
                    .usePlaintext()
                    .build();
            channels.add(channel);
            stubs.add(KVGrpc.newFutureStub(channel));
        }
    }

    /**
     * Get the value associated with a key.
     */
    public CompletableFuture<GetResult> get(String key) {
        GetRequest request = GetRequest.newBuilder()
                .setKey(key)
                .build();

        return callWithRetry(stub -> stub.get(request), lastLeader.get(), 0)
                .thenApply(resp -> {
                    if (resp.getStatus() == Status.OK) {
                        return new GetResult(resp.getValue(), resp.getVersion());
                    } else if (resp.getStatus() == Status.ERR_NO_KEY) {
                        throw KVRaftException.fromStatus(resp.getStatus());
                    }
                    throw KVRaftException.fromStatus(resp.getStatus());
                });
    }

    /**
     * Put a value into the KV store.
     */
    public CompletableFuture<Void> put(String key, String value, long version) {
        PutRequest request = PutRequest.newBuilder()
                .setKey(key)
                .setValue(value)
                .setVersion(version)
                .build();

        return callWithRetry(stub -> stub.put(request), lastLeader.get(), 0)
                .thenAccept(resp -> {
                    if (resp.getStatus() != Status.OK) {
                        throw KVRaftException.fromStatus(resp.getStatus());
                    }
                });
    }

    /**
     * Internal retry mechanism for finding the leader and handling network blips.
     */
    private <T> CompletableFuture<T> callWithRetry(Function<KVGrpc.KVFutureStub, ListenableFuture<T>> rpcCall, int serverIndex, int attempt) {
        CompletableFuture<T> result = new CompletableFuture<>();
        KVGrpc.KVFutureStub stub = stubs.get(serverIndex);

        ListenableFuture<T> future = rpcCall.apply(stub.withDeadlineAfter(5, TimeUnit.SECONDS));

        Futures.addCallback(future, new FutureCallback<T>() {
            @Override
            public void onSuccess(@Nonnull T response) {
                Status status = getStatusFromResponse(response);

                if (status == Status.ERR_WRONG_LEADER) {
                    int hint = getLeaderHintFromResponse(response);
                    retry(hint >= 0 && hint < totalNodes && hint != serverIndex ? hint : (serverIndex + 1) % totalNodes);
                } else {
                    lastLeader.set(serverIndex);
                    result.complete(response);
                }
            }

            @Override
            public void onFailure(@Nonnull Throwable t) {
                retry((serverIndex + 1) % totalNodes);
            }

            private void retry(int nextServerIndex) {
                if (attempt > 0 && attempt % totalNodes == 0) {
                    try { Thread.sleep(50); } catch (InterruptedException ignored) {}
                }
                callWithRetry(rpcCall, nextServerIndex, attempt + 1).whenComplete((res, err) -> {
                    if (err != null) result.completeExceptionally(err);
                    else result.complete(res);
                });
            }
        }, MoreExecutors.directExecutor());

        return result;
    }

    private Status getStatusFromResponse(Object response) {
        if (response instanceof GetResponse) {
            return ((GetResponse) response).getStatus();
        } else if (response instanceof PutResponse) {
            return ((PutResponse) response).getStatus();
        }
        return Status.STATUS_UNSPECIFIED;
    }

    private int getLeaderHintFromResponse(Object response) {
        if (response instanceof GetResponse) {
            return ((GetResponse) response).getLeaderHint();
        } else if (response instanceof PutResponse) {
            return ((PutResponse) response).getLeaderHint();
        }
        return -1;
    }

    @Override
    public void close() {
        for (ManagedChannel channel : channels) {
            channel.shutdown();
            try {
                channel.awaitTermination(2, TimeUnit.SECONDS);
            } catch (InterruptedException e) {
                channel.shutdownNow();
            }
        }
    }
}
