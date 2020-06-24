package main

import (
	"bytes"
	"flag"
	"fmt"
	"k8s.io/client-go/1.4/kubernetes"
	"k8s.io/client-go/1.4/rest"
	"k8s.io/client-go/1.4/tools/clientcmd"
	"os"
	"os/exec"
	"strconv"
	"text/template"
	"time"
)

var (
	kubeconfig      = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	templatePath    = flag.String("template", "/etc/twemproxy/template.yaml", "absolute path to the template file")
	configPath      = flag.String("config", "/etc/twemproxy/config.yaml", "absolute path to the config file")
	twemproxyPath   = flag.String("twemproxy", "/usr/sbin/nutcracker", "absolute path to the twemproxy binary")
	servicePortName = flag.String("port-name", "memcached", "memcached service port name")
)

func applyTemplate(templateFile string, endpoints []string) (string, error) {
	tmpl, err := template.ParseFiles(templateFile)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	tmpl.Execute(&buf, endpoints)
	return buf.String(), nil
}

func writeConfig(content string, configFile string) error {
	file, err := os.OpenFile(configFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(content)

	return err
}

func getEndpoints(clientset *kubernetes.Clientset, serviceName string, namespace string) ([]string, error) {
	endpoints, err := clientset.Core().Endpoints(namespace).Get(serviceName)

	if err != nil {
		return nil, err
	}

	var res []string

	for subidx := range endpoints.Subsets {
		subset := endpoints.Subsets[subidx]
		var port string
		for _, p := range subset.Ports {
			if p.Name == *servicePortName {
				port = strconv.Itoa(int(p.Port))
			}
		}
		if port == "" {
			return nil, err
		}
		for addressidx := range subset.Addresses {
			ip := subset.Addresses[addressidx].IP
			res = append(res, ip+":"+port)
		}
	}

	return res, nil
}

func launchTwemproxy(twemproxyPath string, configPath string) (*TwemproxyInstance, error) {
	cmd := exec.Command(twemproxyPath, "-v", "11", "-c", configPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		return nil, err
	} else {
		finished := make(chan Result)
		go func() {
			finished <- Result{cmd.Wait()}
			close(finished)
		}()
		return &TwemproxyInstance{cmd: cmd, finished: finished}, nil
	}
}

type Result struct {
	err error
}

type TwemproxyInstance struct {
	cmd      *exec.Cmd
	finished chan Result
}

func main() {
	var config *rest.Config

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n\n%s [OPTIONS] [MEMCACHE-ENDPOINT-NAME]\n\n", os.Args[0], os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()
	serviceName := "memcached"

	if len(flag.Args()) != 0 {
		serviceName = flag.Args()[0]
	}

	if *kubeconfig == "" {
		var err error
		if config, err = rest.InClusterConfig(); err != nil {
			panic(err.Error())
		}
	} else {
		var err error
		if config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig); err != nil {
			panic(err.Error())
		}
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	getConfig := func() string {
		namespace := os.Getenv("KUBE_NAMESPACE")
		if namespace == "" {
			namespace = "default"
		}

		endpoints, err := getEndpoints(clientset, serviceName, namespace)
		if err != nil {
			panic(fmt.Sprintf("%s (namespace %s)", err.Error(), namespace))
		}

		if len(endpoints) == 0 {
			return ""
		}

		res, err := applyTemplate(*templatePath, endpoints)
		if err != nil {
			panic(err.Error())
		}
		return res
	}

	prevConfig := getConfig()
	err = writeConfig(prevConfig, *configPath)
	if err != nil {
		panic(err.Error())
	}

	var currentInstance *TwemproxyInstance = nil
	var currentInstanceFinished chan Result = nil

	if prevConfig != "" {
		currentInstance, err = launchTwemproxy(*twemproxyPath, *configPath)
		if err != nil {
			panic(err.Error())
		}
		currentInstanceFinished = currentInstance.finished
	} else {
		fmt.Printf("No enpoints available, waiting to launch twemproxy")
	}

	timer := time.NewTicker(10 * time.Second)

	for {
		select {
		case <-timer.C:
			newConfig := getConfig()
			if newConfig != prevConfig {
				fmt.Printf("Endpoints changed, writing new config\n")
				err = writeConfig(newConfig, *configPath)
				if err != nil {
					panic(err.Error())
				}
				if currentInstance != nil {
					currentInstance.cmd.Process.Kill()
					<-currentInstanceFinished
					currentInstance = nil
					currentInstanceFinished = nil
				}

				if newConfig != "" {
					currentInstance, err = launchTwemproxy(*twemproxyPath, *configPath)
					if err != nil {
						panic(err.Error())
					}
					currentInstanceFinished = currentInstance.finished
				}
			}

			prevConfig = newConfig
		case res := <-currentInstanceFinished:
			fmt.Printf("twemproxy died: %v", res.err)
			return
		}
	}
}
