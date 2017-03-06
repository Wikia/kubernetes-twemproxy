# kubernetes-twemproxy
Process that monitors service endpoints, creates config file based on template and restarts twemproxy when the endpoints changes

```
$ kubernetes-twemproxy -help
Usage of ./kubernetes-twemproxy:

./kubernetes-twemproxy [OPTIONS] [MEMCACHE-ENDPOINT-NAME]

  -alsologtostderr
    	log to standard error as well as files
  -config string
    	absolute path to the config file (default "/etc/twemproxy/config.yaml")
  -kubeconfig string
    	absolute path to the kubeconfig file
  -log_backtrace_at value
    	when logging hits line file:N, emit a stack trace
  -log_dir string
    	If non-empty, write log files in this directory
  -logtostderr
    	log to standard error instead of files
  -stderrthreshold value
    	logs at or above this threshold go to stderr
  -template string
    	absolute path to the template file (default "/etc/twemproxy/template.yaml")
  -twemproxy string
    	absolute path to the twemproxy binary (default "/usr/sbin/nutcracker")
  -v value
    	log level for V logs
  -vmodule value
    	comma-separated list of pattern=N settings for file-filtered logging
```

e.g. to use endpoint my-memcache-endpoint in namespace my-namespace:

`KUBE_NAMESPACE=my-namespace ./kubernetes-twemproxy my-memcache-endpoint`

## Tips

If you use a non-default namespaces, set $KUBE_NAMESPACE, e.g.:

```
          - name: KUBE_NAMESPACE
            valueFrom:
              fieldRef:
                fieldPath: metadata.namespace
```

Since this daemon uses the endpoints api, it's important to include a selector in your memcache service definition so that a corresponding endpoint is created

```
apiVersion: v1
kind: Service
metadata:
  labels:
    app: memcached
  name: memcached
  namespace: fandom
spec:
  ports:
  - port: 11211
    protocol: TCP
    targetPort: 11211
  selector:
    app: memcached
  type: ClusterIP
```
