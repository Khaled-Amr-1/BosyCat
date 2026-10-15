//go:build ignore
#define __TARGET_ARCH_x86
#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_core_read.h>

struct process_metrics {
    __u64 tx_bytes;
    __u64 rx_bytes;
    char comm[16];
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, __u32);
    __type(value, struct process_metrics);
    __uint(max_entries, 10240);
} tracker_map SEC(".maps");

static __always_inline void update_metrics(__u32 tgid, __u64 tx, __u64 rx) {
    struct process_metrics *metrics;
    metrics = bpf_map_lookup_elem(&tracker_map, &tgid);

    if (metrics) {
        __sync_fetch_and_add(&metrics->tx_bytes, tx);
        __sync_fetch_and_add(&metrics->rx_bytes, rx);
    } else {
        struct process_metrics new_metrics = {};
        new_metrics.tx_bytes = tx;
        new_metrics.rx_bytes = rx;
        bpf_get_current_comm(&new_metrics.comm, sizeof(new_metrics.comm));
        bpf_map_update_elem(&tracker_map, &tgid, &new_metrics, BPF_ANY);
    }
}

SEC("kprobe/tcp_sendmsg")
int BPF_KPROBE(trace_tcp_sendmsg, struct sock *sk, struct msghdr *msg, size_t size) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    // استخراج رقم العملية الأب (TGID) لتجميع كل استهلاك الـ Threads تحته
    __u32 tgid = pid_tgid >> 32;

    if (size > 0) {
        update_metrics(tgid, size, 0);
    }
    return 0;
}

SEC("kretprobe/tcp_recvmsg")
int BPF_KRETPROBE(trace_tcp_recvmsg_exit, int ret) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u32 tgid = pid_tgid >> 32;

    if (ret > 0) {
        update_metrics(tgid, 0, ret);
    }
    return 0;
}

// --------------------------------------------------------
// تتبع إرسال واستقبال UDP
// --------------------------------------------------------

SEC("kprobe/udp_sendmsg")
int BPF_KPROBE(trace_udp_sendmsg, struct sock *sk, struct msghdr *msg, size_t size) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u32 tgid = pid_tgid >> 32;

    if (size > 0) {
        update_metrics(tgid, size, 0);
    }
    return 0;
}

SEC("kretprobe/udp_recvmsg")
int BPF_KRETPROBE(trace_udp_recvmsg_exit, int ret) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u32 tgid = pid_tgid >> 32;

    if (ret > 0) {
        update_metrics(tgid, 0, ret);
    }
    return 0;
}

char _license[] SEC("license") = "GPL";