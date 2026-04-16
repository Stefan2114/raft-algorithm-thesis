package com.kvraft;

/**
 * Represents the result of a Get operation from the KVRaft cluster.
 */
public final class GetResult {
    private final String value;
    private final long version;

    public GetResult(String value, long version) {
        this.value = value;
        this.version = version;
    }

    public String getValue() {
        return value;
    }

    public long getVersion() {
        return version;
    }

    @Override
    public String toString() {
        return "GetResult{" +
                "value='" + value + '\'' +
                ", version=" + version +
                '}';
    }
}
