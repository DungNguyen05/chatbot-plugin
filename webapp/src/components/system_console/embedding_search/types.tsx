// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

export interface UpstreamConfig {
    type: string;
    parameters: Record<string, unknown>;
}

export interface ChunkingOptions {
    chunkSize: number;
    chunkOverlap: number;
    minChunkSize: number;
    chunkingStrategy: string;
}

export interface EmbeddingSearchConfig {
    type: string;
    vectorStore: UpstreamConfig;
    embeddingProvider: UpstreamConfig;
    parameters: Record<string, unknown>;
    dimensions: number;
    chunkingOptions?: ChunkingOptions;
}

// Match the server's JobStatus struct field names
export interface JobStatusType {
    status: string; // 'running' | 'completed' | 'failed' | 'canceled' | 'no_job'
    error?: string;
    started_at: string; // ISO string from server's time.Time
    completed_at?: string;
    processed_rows: number;
    total_rows: number;
    last_k_posts?: number; // Number of recent posts to reindex (0 means all)
    full_reindex?: boolean; // Whether this is a full reindex
}

export interface StatusMessageType {
    success?: boolean;
    message?: string;
}

// Request payload for reindexing
export interface ReindexRequest {
    lastKPosts?: number; // If 0 or not provided, reindex all posts
    fullReindex?: boolean; // Explicitly request full reindex
}