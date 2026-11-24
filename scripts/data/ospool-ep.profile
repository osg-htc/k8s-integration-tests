abi <abi/4.0>,
include <tunables/global>

profile docker-ep flags=(attach_disconnected, mediate_deleted) {
  network,
  capability,
  file,
  umount,

  # Allow unprivileged user namespace creation. This allows the EP
  # to apply container isolation mechanisms between the outer Docker container and
  # inner Singularity container.
  userns,

  # Host (privileged) processes may send signals to container processes.
  signal (receive) peer=unconfined,
  # runc may send signals to container processes (for "docker stop").
  signal (receive) peer=runc,
  # crun may send signals to container processes (for "docker stop" when used with crun OCI runtime).
  signal (receive) peer=crun,
  # dockerd may send signals to container processes (for "docker kill").
  signal (receive) peer=unconfined,
  # Container processes may send signals amongst themselves.
  signal (send,receive) peer=docker-ep,

  deny @{PROC}/* w,   # deny write for all files directly in /proc (not in a subdir)
  # deny write to files not in /proc/<number>/** or /proc/sys/**
  deny @{PROC}/{[^1-9],[^1-9][^0-9],[^1-9s][^0-9y][^0-9s],[^1-9][^0-9][^0-9][^0-9/]*}/** w,
  deny @{PROC}/sys/[^k]** w,  # deny /proc/sys except /proc/sys/k* (effectively /proc/sys/kernel)
  deny @{PROC}/sys/kernel/{?,??,[^s][^h][^m]**} w,  # deny everything except shm* in /proc/sys/kernel/
  deny @{PROC}/sysrq-trigger rwklx,
  deny @{PROC}/kcore rwklx,

  # Allow mount(2) for Singularity's container setup.
  # The default Docker profile contains "deny mount".
  mount,

  # Allow pivot_root. This allows cvmfsexec inside the EP container to expose
  # the mounted CVMFS repos at the appropriate location for the pilot.
  pivot_root,

  deny /sys/[^f]*/** wklx,
  deny /sys/f[^s]*/** wklx,
  deny /sys/fs/[^c]*/** wklx,
  deny /sys/fs/c[^g]*/** wklx,
  deny /sys/fs/cg[^r]*/** wklx,
  deny /sys/firmware/** rwklx,
  deny /sys/devices/virtual/powercap/** rwklx,
  deny /sys/kernel/security/** rwklx,

  # Allow processes within the container to trace each other,
  # provided all other LSM and yama settings allow it.
  ptrace (trace,tracedby,read,readby) peer=docker-ep,
}
