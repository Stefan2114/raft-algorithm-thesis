package com.kvraft;

import com.kvraft.kv.v1.Status;

/**
 * Exception thrown when the KVRaft cluster returns a domain error.
 */
public class KVRaftException extends RuntimeException {
    private final Status status;

    public KVRaftException(Status status, String message) {
        super(message);
        this.status = status;
    }

    public Status getStatus() {
        return status;
    }

    public static KVRaftException fromStatus(Status status) {
        return new KVRaftException(status, "KVRaft operation failed with status: " + status);
    }
}
