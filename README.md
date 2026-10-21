# About

This is an example that toggles the power of a remote system using the [Intel Active_Management_Technology (AMT)](https://en.wikipedia.org/wiki/Intel_Active_Management_Technology) remote api.

This uses the [device-management-toolkit/go-wsman-messages library](https://github.com/device-management-toolkit/go-wsman-messages) to communicate with the remote system.

## Usage

Provision AMT as described in [rgl/intel-amt-notes](https://github.com/rgl/intel-amt-notes).

**NB** To be able to do a Graceful Shutdown/Reboot you must have the [Intel LMS (Local Manageability Service)](https://github.com/intel/lms) running in the OS.

Install `go` and `xmllint`.

Build and see the help:

```bash
go build
./intel-amt-toggle-power-go-example --help
```

Execute the example, it will toggle the power of the system:

```sh
./intel-amt-toggle-power-go-example \
    --host 192.168.8.22 \
    --username admin \
    --password the-admin-password
```

## Alternatives

* [bmclib Intel AMT provider](https://github.com/bmc-toolbox/bmclib/tree/main/providers/intelamt).
