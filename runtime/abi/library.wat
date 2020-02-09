;; Copyright (c) 2019 Timo Savola. All rights reserved.
;; Use of this source code is governed by a BSD-style
;; license that can be found in the LICENSE file.

;; library.go can be updated by running go generate.

(module
 (import "rt" "debug" (func $rt_debug (param i32 i32)))
 (import "rt" "nop" (func $reserved))
 (import "rt" "poll" (func $rt_poll (param i32 i32) (result i32)))
 (import "rt" "random" (func $rt_random (result i32)))
 (import "rt" "read" (func $rt_read (param i32 i32) (result i32)))
 (import "rt" "stop" (func $rt_stop (param i32) (result i64)))
 (import "rt" "time" (func $rt_time (param i32) (result i64)))
 (import "rt" "write" (func $rt_write (param i32 i32) (result i32)))

 (memory 0)

 (func (export "args_get")
       (param $argv i32)    ;; 0 * 4 bytes.
       (param $argvbuf i32) ;; 0 bytes.
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (i32.const 0))

 (func (export "args_sizes_get")
       (param $argc_ptr i32)        ;; 4 bytes.
       (param $argvbufsize_ptr i32) ;; 4 bytes.
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (i32.store (get_local $argc_ptr) (i32.const 0))
       (i32.store (get_local $argvbufsize_ptr) (i32.const 0))
       (i32.const 0))

 (func (export "clock_res_get")
       (param $id i32)
       (param $buf i32) ;; 8 bytes.
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (if (i32.ge_u (get_local $id) (i32.const 4))
           (return (i32.const 28))) ;; EINVAL
       (i64.store (get_local $buf) (i64.const 1000000000)) ;; Worst-case scenario.
       (i32.const 0))

 (func (export "clock_time_get")
       (param $id i32)
       (param $precision i64)
       (param $buf i32) ;; 8 bytes.
       (result i32)
       (local $time i64)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (if (i32.ge_u (get_local $id) (i32.const 4))
           (return (i32.const 28))) ;; EINVAL
       (block $done
         (block $supported
           (loop $retry
                 (br_if $supported (i32.lt_u (get_local $id) (i32.const 2)))
                 (set_local $time (call $rt_stop (i32.const 127))) ;; ABI deficiency.
                 (br_if $done (i64.ne (get_local $time) (i64.const 0)))
                 (br $retry))) ;; Implicit call site.
         (set_local $time (call $rt_time (i32.add (get_local $id) (i32.const 5))))) ;; Coarse
       (i64.store (get_local $buf) (get_local $time))
       (i32.const 0))

 (func (export "environ_get")
       (param $env i32)    ;; 3 * 4 bytes.
       (param $envbuf i32) ;; 56 bytes.
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (i64.store offset=0  (get_local $envbuf) (i64.const 0x4942415f45544147)) ;; "GATE_ABI"
       (i64.store offset=8  (get_local $envbuf) (i64.const 0x4e4f49535245565f)) ;; "_VERSION"
       (i64.store offset=16 (get_local $envbuf) (i64.const 0x5f4554414700303d)) ;; "=0\0GATE_"
       (i64.store offset=24 (get_local $envbuf) (i64.const 0x54414700343d4446)) ;; "FD=4\0GAT"
       (i64.store offset=32 (get_local $envbuf) (i64.const 0x45535f58414d5f45)) ;; "E_MAX_SE"
       (i64.store offset=40 (get_local $envbuf) (i64.const 0x3d455a49535f444e)) ;; "ND_SIZE="
       (i64.store offset=48 (get_local $envbuf) (i64.const 0x0000003633353536)) ;; "65536\0\0\0"
       (i32.store offset=0 (get_local $env) (get_local $envbuf))
       (i32.store offset=4 (get_local $env) (i32.add (get_local $envbuf) (i32.const 19)))
       (i32.store offset=8 (get_local $env) (i32.add (get_local $envbuf) (i32.const 29)))
       (i32.const 0))

 (func (export "environ_sizes_get")
       (param $envlen_ptr i32)     ;; 4 bytes.
       (param $envbufsize_ptr i32) ;; 4 bytes.
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (i32.store (get_local $envlen_ptr) (i32.const 3))
       (i32.store (get_local $envbufsize_ptr) (i32.const 56))
       (i32.const 0))

 (func (export "io")
       (param $recv i32)      ;; recvlen * 8 bytes.
       (param $recvlen i32)
       (param $nrecv_ptr i32) ;; 4 bytes.
       (param $send i32)      ;; sendlen * 8 bytes.
       (param $sendlen i32)
       (param $nsent_ptr i32) ;; 4 bytes.
       (param $flags i32)
       (local $filter i32)
       (local $xfer i32)
       (local $buflen i32)
       (local $n i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (set_local $filter (i32.const 0x5)) ;; POLLIN | POLLOUT
       (if (i32.and (get_local $flags) (i32.const 0x1)) ;; GATE_IO_WAIT
           (set_local $filter (call $rt_poll
                                    (i32.const 0x1) ;; POLLIN
                                    (if (result i32)
                                        (get_local $sendlen)
                                        (i32.const 0x4) ;; POLLOUT
                                        (i32.const 0)))))
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (if (i32.and (get_local $filter) (i32.const 0x4)) ;; POLLOUT
           (block $break
             (loop $cont
                   (br_if $break (i32.eqz (get_local $sendlen)))
                   (set_local $buflen (i32.load offset=4 (get_local $send)))
                   (set_local $n (call $rt_write
                                       (i32.load offset=0 (get_local $send))
                                       (get_local $buflen)))
                   (set_local $xfer (i32.add (get_local $xfer) (get_local $n)))
                   (br_if $break (i32.ne (get_local $n) (get_local $buflen)))
                   (set_local $send (i32.add (get_local $send) (i32.const 8)))
                   (set_local $sendlen (i32.sub (get_local $sendlen) (i32.const 1)))
                   (br $cont)))) ;; Implicit call site.
       (if (get_local $nsent_ptr)
           (i32.store (get_local $nsent_ptr) (get_local $xfer)))
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (set_local $xfer (i32.const 0))
       (if (i32.and (get_local $filter) (i32.const 0x1)) ;; POLLIN
           (block $break
             (loop $cont
                   (br_if $break (i32.eqz (get_local $recvlen)))
                   (set_local $buflen (i32.load offset=4 (get_local $recv)))
                   (set_local $n (call $rt_read
                                       (i32.load offset=0 (get_local $recv))
                                       (get_local $buflen)))
                   (set_local $xfer (i32.add (get_local $xfer) (get_local $n)))
                   (br_if $break (i32.ne (get_local $n) (get_local $buflen)))
                   (set_local $recv (i32.add (get_local $recv) (i32.const 8)))
                   (set_local $recvlen (i32.sub (get_local $recvlen) (i32.const 1)))
                   (br $cont)))) ;; Implicit call site.
       (if (get_local $nrecv_ptr)
           (i32.store (get_local $nrecv_ptr) (get_local $xfer)))
       (block (br_if 0 (i32.const 1)) (call $reserved)))

 (func (export "fd")
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (i32.const 4))

 (func (export "fd_close")
       (param $fd i32)
       (result i32)
       (local $result i64)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (if (i32.or (i32.eq (get_local $fd) (i32.const 3))
                   (i32.ge_u (get_local $fd) (i32.const 5)))
           (return (i32.const 8))) ;; EBADF
       (loop $retry
             (set_local $result (call $rt_stop (i32.const 127))) ;; ABI deficiency.
             (if (i64.eq (get_local $result) (i64.const 0))
                 (br $retry))) ;; Implicit call site.
       (i32.wrap/i64 (get_local $result)))

 (func (export "fd_fdstat_get")
       (param $fd i32)
       (param $buf i32) ;; 24 bytes.
       (result i32)
       (local $flags i32)
       (local $rights i64)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (if (i32.or (i32.eq (get_local $fd) (i32.const 3))
                   (i32.ge_u (get_local $fd) (i32.const 5)))
           (return (i32.const 8))) ;; EBADF
       (if (i32.or (i32.eq (get_local $fd) (i32.const 1))  ;; stdout
                   (i32.eq (get_local $fd) (i32.const 2))) ;; stderr
           (set_local $rights (i64.const 0x40))) ;; RIGHT_FD_WRITE
       (if (i32.eq (get_local $fd) (i32.const 4)) ;; Gate fd
           (then (set_local $flags (i32.const 0x4))     ;; FDFLAG_NONBLOCK
                 (set_local $rights (i64.const 0x42)))) ;; RIGHT_FD_READ | RIGHT_FD_WRITE
       (i32.store8  offset=0  (get_local $buf) (i32.const 0))       ;; FILETYPE_UNKNOWN
       (i32.store16 offset=2  (get_local $buf) (get_local $flags))  ;; fs_flags
       (i64.store   offset=8  (get_local $buf) (get_local $rights)) ;; fs_rights_base
       (i64.store   offset=16 (get_local $buf) (i64.const 0))       ;; fs_rights_inheriting
       (i32.const 0))

 (func (export "fd_fdstat_set_flags")
       (param $fd i32)
       (param $flags i32)
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (if (i32.or (i32.eq (get_local $fd) (i32.const 3))
                   (i32.ge_u (get_local $fd) (i32.const 5)))
           (return (i32.const 8))) ;; EBADF
       (i32.const 63)) ;; EPERM

 (func (export "fd_fdstat_set_rights")
       (param $fd i32)
       (param $base i64)
       (param $inheriting i64)
       (result i32)
       (local $result i64)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (if (i32.or (i32.eq (get_local $fd) (i32.const 3))
                   (i32.ge_u (get_local $fd) (i32.const 5)))
           (return (i32.const 8))) ;; EBADF
       (if (i64.ne (get_local $inheriting) (i64.const 0))
           (return (i32.const 76))) ;; ENOTCAPABLE
       (if (i32.eqz (get_local $fd)) ;; stdin
           (if (i64.eq (get_local $base) (i64.const 0))
               (then (return (i32.const 0)))
               (else (return (i32.const 76))))) ;; ENOTCAPABLE
       (if (i32.or (i32.eq (get_local $fd) (i32.const 1))  ;; stdout
                   (i32.eq (get_local $fd) (i32.const 2))) ;; stderr
           (then (if (i64.eq (get_local $base) (i64.const 0x40)) ;; RIGHT_FD_WRITE
                     (return (i32.const 0)))
                 (if (i64.ne (get_local $base) (i64.const 0))
                     (return (i32.const 76))))) ;; ENOTCAPABLE
       (if (i32.eq (get_local $fd) (i32.const 4)) ;; Gate fd
           (then (if (i64.eq (get_local $base) (i64.const 0x42)) ;; RIGHT_FD_READ | RIGHT_FD_WRITE
                     (return (i32.const 0)))
                 (if (i64.ne (i64.and (get_local $base) (i64.const 0xffffffffffffffbd)) ;; ~0x42
                             (i64.const 0))
                     (return (i32.const 76))))) ;; ENOTCAPABLE
       (loop $retry
             (set_local $result (call $rt_stop (i32.const 127))) ;; ABI deficiency.
             (if (i64.eq (get_local $result) (i64.const 0))
                 (br $retry))) ;; Implicit call site.
       (i32.wrap/i64 (get_local $result)))

 (func (export "fd_filestat_get")
       (param $fd i32)
       (param $buf i32) ;; 56 bytes.
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (if (i32.or (i32.eq (get_local $fd) (i32.const 3))
                   (i32.ge_u (get_local $fd) (i32.const 5)))
           (return (i32.const 8))) ;; EBADF
       (i64.store  offset=0  (get_local $buf) (i64.const 0)) ;; st_dev
       (i64.store  offset=8  (get_local $buf) (i64.const 0)) ;; st_ino
       (i32.store8 offset=16 (get_local $buf) (i32.const 0)) ;; FILETYPE_UNKNOWN
       (i32.store  offset=20 (get_local $buf) (i32.const 0)) ;; st_nlink
       (i32.store  offset=24 (get_local $buf) (i32.const 0)) ;; st_size
       (i64.store  offset=32 (get_local $buf) (i64.const 0)) ;; st_atim
       (i64.store  offset=40 (get_local $buf) (i64.const 0)) ;; st_mtim
       (i64.store  offset=48 (get_local $buf) (i64.const 0)) ;; st_ctim
       (i32.const 0))

 (func (export "fd_read")
       (param $fd i32)
       (param $iov i32)       ;; iovlen * 8 bytes.
       (param $iovlen i32)
       (param $nread_ptr i32) ;; 4 bytes.
       (result i32)
       (local $nread i32)
       (local $buflen i32)
       (local $n i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (if (i32.or (i32.eq (get_local $fd) (i32.const 3))
                   (i32.ge_u (get_local $fd) (i32.const 5)))
           (return (i32.const 8))) ;; EBADF
       (if (i32.eqz (get_local $fd)) ;; stdin
           (return (i32.const 63))) ;; EPERM
       (if (i32.ne (get_local $fd) (i32.const 4)) ;; Gate fd
           (return (i32.const 28))) ;; EINVAL
       (block $break
         (loop $cont
               (br_if $break (i32.eqz (get_local $iovlen)))
               (set_local $buflen (i32.load offset=4 (get_local $iov)))
               (set_local $n (call $rt_read
                                   (i32.load offset=0 (get_local $iov))
                                   (get_local $buflen)))
               (set_local $nread (i32.add (get_local $nread) (get_local $n)))
               (br_if $break (i32.ne (get_local $n) (get_local $buflen)))
               (set_local $iov (i32.add (get_local $iov) (i32.const 8)))
               (set_local $iovlen (i32.sub (get_local $iovlen) (i32.const 1)))
               (br $cont))) ;; Implicit call site.
       (i32.store (get_local $nread_ptr) (get_local $nread))
       (if (get_local $nread)
           (return (i32.const 0)))
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (i32.const 6)) ;; EAGAIN

 (func (export "fd_renumber")
       (param $from i32)
       (param $to i32)
       (result i32)
       (local $result i64)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (if (i32.or (i32.or (i32.eq (get_local $from) (i32.const 3))
                           (i32.ge_u (get_local $from) (i32.const 5)))
                   (i32.or (i32.eq (get_local $to) (i32.const 3))
                           (i32.ge_u (get_local $to) (i32.const 5))))
           (return (i32.const 8))) ;; EBADF
       (if (i32.eq (get_local $from) (get_local $to))
           (return (i32.const 0)))
       (loop $retry
             (set_local $result (call $rt_stop (i32.const 127))) ;; ABI deficiency.
             (if (i64.eq (get_local $result) (i64.const 0))
                 (br $retry))) ;; Implicit call site.
       (i32.wrap/i64 (get_local $result)))

 (func (export "fd_stub_i32")
       (param $fd i32)
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (if (i32.or (i32.eq (get_local $fd) (i32.const 3))
                   (i32.ge_u (get_local $fd) (i32.const 5)))
           (return (i32.const 8))) ;; EBADF
       (i32.const 70)) ;; ESPIPE

 (func (export "fd_stub_i32i32")
       (param $fd i32)
       (param i32)
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (if (i32.or (i32.eq (get_local $fd) (i32.const 3))
                   (i32.ge_u (get_local $fd) (i32.const 5)))
           (return (i32.const 8))) ;; EBADF
       (i32.const 70)) ;; ESPIPE

 (func (export "fd_stub_i32i64")
       (param $fd i32)
       (param i64)
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (if (i32.or (i32.eq (get_local $fd) (i32.const 3))
                   (i32.ge_u (get_local $fd) (i32.const 5)))
           (return (i32.const 8))) ;; EBADF
       (i32.const 70)) ;; ESPIPE

 (func (export "fd_stub_i32i32i32")
       (param $fd i32)
       (param i32 i32)
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (if (i32.or (i32.eq (get_local $fd) (i32.const 3))
                   (i32.ge_u (get_local $fd) (i32.const 5)))
           (return (i32.const 8))) ;; EBADF
       (i32.const 70)) ;; ESPIPE

 (func (export "fd_stub_i32i64i64")
       (param $fd i32)
       (param i64 i64)
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (if (i32.or (i32.eq (get_local $fd) (i32.const 3))
                   (i32.ge_u (get_local $fd) (i32.const 5)))
           (return (i32.const 8))) ;; EBADF
       (i32.const 70)) ;; ESPIPE

 (func (export "fd_stub_i32i64i32i32")
       (param $fd i32)
       (param i64 i32 i32)
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (if (i32.or (i32.eq (get_local $fd) (i32.const 3))
                   (i32.ge_u (get_local $fd) (i32.const 5)))
           (return (i32.const 8))) ;; EBADF
       (i32.const 70)) ;; ESPIPE

 (func (export "fd_stub_i32i64i64i32")
       (param $fd i32)
       (param i64 i64 i32)
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (if (i32.or (i32.eq (get_local $fd) (i32.const 3))
                   (i32.ge_u (get_local $fd) (i32.const 5)))
           (return (i32.const 8))) ;; EBADF
       (i32.const 70)) ;; ESPIPE

 (func (export "fd_stub_i32i32i32i64i32")
       (param $fd i32)
       (param i32 i32 i64 i32)
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (if (i32.or (i32.eq (get_local $fd) (i32.const 3))
                   (i32.ge_u (get_local $fd) (i32.const 5)))
           (return (i32.const 8))) ;; EBADF
       (i32.const 70)) ;; ESPIPE

 (func (export "fd_write")
       (param $fd i32)
       (param $iov i32)          ;; iovlen * 8 bytes.
       (param $iovlen i32)
       (param $nwritten_ptr i32) ;; 4 bytes.
       (result i32)
       (local $nwritten i32)
       (local $buflen i32)
       (local $n i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (if (i32.or (i32.eq (get_local $fd) (i32.const 3))
                   (i32.ge_u (get_local $fd) (i32.const 5)))
           (return (i32.const 8))) ;; EBADF
       (if (i32.eqz (get_local $fd)) ;; stdin
           (return (i32.const 28))) ;; EINVAL
       (block $break
         (if (i32.eq (get_local $fd) (i32.const 4))
             (then (loop $cont
                         (br_if $break (i32.eqz (get_local $iovlen)))
                         (set_local $buflen (i32.load offset=4 (get_local $iov)))
                         (set_local $n (call $rt_write
                                             (i32.load offset=0 (get_local $iov))
                                             (get_local $buflen)))
                         (set_local $nwritten (i32.add (get_local $nwritten) (get_local $n)))
                         (br_if $break (i32.ne (get_local $n) (get_local $buflen)))
                         (set_local $iov (i32.add (get_local $iov) (i32.const 8)))
                         (set_local $iovlen (i32.sub (get_local $iovlen) (i32.const 1)))
                         (br $cont))) ;; Implicit call site.
             (else (loop $cont
                         (br_if $break (i32.eqz (get_local $iovlen)))
                         (set_local $buflen (i32.load offset=4 (get_local $iov)))
                         (call $rt_debug
                               (i32.load offset=0 (get_local $iov))
                               (get_local $buflen))
                         (set_local $nwritten (i32.add (get_local $nwritten) (get_local $buflen)))
                         (set_local $iov (i32.add (get_local $iov) (i32.const 8)))
                         (set_local $iovlen (i32.sub (get_local $iovlen) (i32.const 1)))
                         (br $cont))))) ;; Implicit call site.
       (i32.store (get_local $nwritten_ptr) (get_local $nwritten))
       (if (get_local $nwritten)
           (return (i32.const 0)))
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (i32.const 6)) ;; EAGAIN

 (func (export "path_stub_i32i32i32")
       (param i32 i32 i32)
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (i32.const 44)) ;; ENOENT

 (func (export "path_stub_i32i32i32i32i32")
       (param i32 i32 i32 i32 i32)
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (i32.const 44)) ;; ENOENT

 (func (export "path_stub_i32i32i32i32i32i32")
       (param i32 i32 i32 i32 i32 i32)
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (i32.const 44)) ;; ENOENT

 (func (export "path_stub_i32i32i32i32i32i32i32")
       (param i32 i32 i32 i32 i32 i32 i32)
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (i32.const 44)) ;; ENOENT

 (func (export "path_stub_i32i32i32i32i64i64i32")
       (param i32 i32 i32 i32 i64 i64 i32)
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (i32.const 44)) ;; ENOENT

 (func (export "path_stub_i32i32i32i32i32i64i64i32i32")
       (param i32 i32 i32 i32 i32 i64 i64 i32 i32)
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (i32.const 44)) ;; ENOENT

 (func (export "poll_oneoff")
       (param $sub_ptr i32) ;; nsub * 56 bytes.
       (param $out_ptr i32) ;; nsub * 32 bytes.
       (param $nsub i32)
       (param $nout_ptr i32)
       (result i32)
       (local $type i32)
       (local $fd i32)
       (local $reading i32) ;; poll(2) events
       (local $writing i32) ;; poll(2) events
       (local $read_userdata i64)
       (local $write_userdata i64)
       (local $result i32) ;; poll(2) revents
       (local $nout i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (block $break
         (loop $cont
               (br_if $break (i32.eqz (get_local $nsub)))
               (set_local $type (i32.load8_u offset=8 (get_local $sub_ptr)))
               (if (i32.ge_u (get_local $type) (i32.const 3))
                   (return (i32.const 28))) ;; EINVAL
               (if (i32.eqz (get_local $type)) ;; EVENTTYPE_CLOCK
                   (then (drop (call $rt_stop (i32.const 127))) ;; ABI deficiency.
                         (br $cont))) ;; Implicit call site.
               (set_local $fd (i32.load offset=16 (get_local $sub_ptr)))
               (if (i32.eq (get_local $fd) (i32.const 4))
                   (then (if (i32.eq (get_local $type) (i32.const 1)) ;; EVENTTYPE_FD_READ
                             (then (set_local $reading (i32.const 0x1)) ;; POLLIN
                                   (set_local $read_userdata (i64.load (get_local $sub_ptr)))))
                         (if (i32.eq (get_local $type) (i32.const 2)) ;; EVENTTYPE_FD_WRITE
                             (then (set_local $writing (i32.const 0x4)) ;; POLLOUT
                                   (set_local $write_userdata (i64.load (get_local $sub_ptr))))))
                   (else (if (i32.ge_u (get_local $fd) (i32.const 3))
                             (then (i64.store   offset=0 (get_local $out_ptr) (i64.load (get_local $sub_ptr))) ;; userdata
                                   (i32.store16 offset=8 (get_local $out_ptr) (i32.const 8))                   ;; EBADF
                                   (set_local $out_ptr (i32.add (get_local $out_ptr) (i32.const 32)))
                                   (set_local $nout (i32.add (get_local $nout) (i32.const 1)))))))
               (if (i32.and (i32.or (i32.eq (get_local $fd) (i32.const 1))  ;; stdout
                                    (i32.eq (get_local $fd) (i32.const 2))) ;; stderr
                            (i32.eq (get_local $type) (i32.const 2))) ;; EVENTTYPE_FD_WRITE
                   (then (i64.store   offset=0  (get_local $out_ptr) (i64.load (get_local $sub_ptr))) ;; userdata
                         (i32.store16 offset=8  (get_local $out_ptr) (i32.const 0))                   ;; error
                         (i32.store8  offset=10 (get_local $out_ptr) (i32.const 2))                   ;; EVENTTYPE_FD_WRITE
                         (i64.store   offset=16 (get_local $out_ptr) (i64.const 1))                   ;; fd nbytes
                         (i32.store16 offset=24 (get_local $out_ptr) (i32.const 0))                   ;; fd flags
                         (set_local $out_ptr (i32.add (get_local $out_ptr) (i32.const 32)))
                         (set_local $nout (i32.add (get_local $nout) (i32.const 1)))))
               (set_local $sub_ptr (i32.add (get_local $sub_ptr) (i32.const 56)))
               (set_local $nsub (i32.sub (get_local $nsub) (i32.const 1)))
               (br $cont))) ;; Implicit call site.
       (set_local $result (call $rt_poll
                                (get_local $reading)
                                (get_local $writing)))
       (if (i32.and (get_local $result) (i32.const 0x1)) ;; POLLIN
           (then (i64.store   offset=0  (get_local $out_ptr) (get_local $read_userdata))
                 (i32.store16 offset=8  (get_local $out_ptr) (i32.const 0))     ;; error
                 (i32.store8  offset=10 (get_local $out_ptr) (i32.const 1))     ;; EVENTTYPE_FD_READ
                 (i64.store   offset=16 (get_local $out_ptr) (i64.const 65536)) ;; fd nbytes
                 (i32.store16 offset=24 (get_local $out_ptr) (i32.const 0))     ;; fd flags
                 (set_local $out_ptr (i32.add (get_local $out_ptr) (i32.const 32)))
                 (set_local $nout (i32.add (get_local $nout) (i32.const 1)))))
       (if (i32.and (get_local $result) (i32.const 0x4)) ;; POLLOUT
           (then (i64.store   offset=0  (get_local $out_ptr) (get_local $write_userdata))
                 (i32.store16 offset=8  (get_local $out_ptr) (i32.const 0))     ;; error
                 (i32.store8  offset=10 (get_local $out_ptr) (i32.const 2))     ;; EVENTTYPE_FD_WRITE
                 (i64.store   offset=16 (get_local $out_ptr) (i64.const 65536)) ;; fd nbytes
                 (i32.store16 offset=24 (get_local $out_ptr) (i32.const 0))     ;; fd flags
                 (set_local $out_ptr (i32.add (get_local $out_ptr) (i32.const 32)))
                 (set_local $nout (i32.add (get_local $nout) (i32.const 1)))))
       (i32.store (get_local $nout_ptr) (get_local $nout))
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (i32.const 0))

 (func (export "proc_exit")
       (param $status i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (set_local $status (i32.or (i32.ne (get_local $status) (i32.const 0)) ;; Result
                                  (i32.const 0x2)))                          ;; Terminated
       (loop $cont
             (drop (call $rt_stop (get_local $status)))
             (br $cont))) ;; Implicit call site.

 (func (export "proc_raise")
       (param $signal i32)
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (i32.wrap/i64 (call $rt_stop (i32.const 127)))) ;; ABI deficiency.

 (func (export "random_get")
       (param $buf i32)
       (param $len i32)
       (result i32)
       (local $result i64)
       (local $value i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (block $break
         (loop $cont
               (if (i32.eqz (get_local $len))
                   (br $break))
               (set_local $value (call $rt_random))
               (if (i32.ge_s (get_local $value) (i32.const 0))
                   (then (i32.store8 (get_local $buf) (get_local $value))
                         (set_local $buf (i32.add (get_local $buf) (i32.const 1)))
                         (set_local $len (i32.sub (get_local $len) (i32.const 1))))
                   (else (set_local $result (call $rt_stop (i32.const 127))) ;; ABI deficiency.
                         (if (i64.ne (get_local $result) (i64.const 0))
                             (br $break))))
               (br $cont))) ;; Implicit call site.
       (i32.wrap/i64 (get_local $result)))

 (func (export "sched_yield")
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (i32.const 0))

 (func (export "sock_recv")
       (param $fd i32)
       (param i32 i32 i32 i32 i32)
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (if (i32.or (i32.eq (get_local $fd) (i32.const 3))
                   (i32.ge_u (get_local $fd) (i32.const 5)))
           (return (i32.const 8))) ;; EBADF
       (i32.const 57)) ;; ENOTSOCK

 (func (export "sock_send")
       (param $fd i32)
       (param i32 i32 i32 i32)
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (if (i32.or (i32.eq (get_local $fd) (i32.const 3))
                   (i32.ge_u (get_local $fd) (i32.const 5)))
           (return (i32.const 8))) ;; EBADF
       (i32.const 57)) ;; ENOTSOCK

 (func (export "sock_shutdown")
       (param $fd i32)
       (param i32)
       (result i32)
       (block (br_if 0 (i32.const 1)) (call $reserved))
       (if (i32.or (i32.eq (get_local $fd) (i32.const 3))
                   (i32.ge_u (get_local $fd) (i32.const 5)))
           (return (i32.const 8))) ;; EBADF
       (i32.const 57)) ;; ENOTSOCK

 )
