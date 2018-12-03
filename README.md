# _kured-toujours_

`kured-toujours` is a simple daemon used to ask for a reboot after some uptime.

## TL;DR

```
# Reboot every week
kured-toujours --period 168h
[.........]
File /var/run/reboot-required touched
```

## Motivation

[kubernetes](https://kubernetes.io/) nodes may need to be restarted at some point.
Upgrade, cleanup etc.

[kured](https://github.com/weaveworks/kured) is really awesome to operate nodes when packages upgrade are done.
But we may want to upgrade periodically even without upgrade.

## Deployment

```
kubectl apply -f https://raw.githubusercontent.com/barpilot/kured-toujours/master/daemonset.yaml
```
