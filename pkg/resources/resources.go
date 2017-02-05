package resources

import (
	_ "fmt"

	"reflect"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/pkg/util/intstr"

	"github.com/errordeveloper/kubegen/pkg/util"
)

func (i *Container) maybeAddEnvVars(container *v1.Container) {
	if len(i.Env) == 0 {
		return
	}

	keys := []string{}
	for k, _ := range i.Env {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	env := []v1.EnvVar{}
	for _, j := range keys {
		for k, v := range i.Env {
			if k == j {
				env = append(env, v1.EnvVar{Name: k, Value: v})
			}
		}
	}
	container.Env = env
}

func (i *Container) Convert() v1.Container {
	container := v1.Container{Name: i.Name, Image: i.Image}

	i.maybeAddEnvVars(&container)

	// you'd think the types could be simply converted,
	// but it turns out they won't because tags are different...
	// Fortunatelly, this has changed in Go1.8!
	//container.Ports = []v1.ContainerPort(i.Ports)
	for _, port := range i.Ports {
		container.Ports = append(container.Ports, v1.ContainerPort(port))
	}

	for _, volumeMount := range i.Mounts {
		container.VolumeMounts = append(container.VolumeMounts, v1.VolumeMount(volumeMount))
	}

	if i.LivenessProbe != nil {
		container.LivenessProbe = i.LivenessProbe.Convert(i.Ports)
	}

	if i.ReadinessProbe != nil {
		container.ReadinessProbe = i.ReadinessProbe.Convert(i.Ports)
	}

	return container
}

func exclusiveNonNil(args ...interface{}) *int {
	count := 0
	index := 0
	for k, v := range args {
		if !reflect.ValueOf(v).IsNil() {
			count = count + 1
			index = k
		}
	}

	if count == 0 || count > 1 {
		return nil
	} else {
		return &index
	}
}

func (i *Probe) Convert(ports []ContainerPort) *v1.Probe {
	probe := v1.Probe{Handler: v1.Handler{}}

	defaultPort := intstr.IntOrString{}

	// pick the first port by default
	if len(ports) > 0 {
		defaultPort = intstr.FromString(ports[0].Name)
	}

	whichHandler := exclusiveNonNil(i.Handler.Exec, i.Handler.HTTPGet, i.Handler.TCPSocket)
	if whichHandler != nil {
		switch *whichHandler {
		case 0:
			a := v1.ExecAction(*i.Handler.Exec)
			probe.Handler.Exec = &a
		case 1:
			a := v1.HTTPGetAction{Port: defaultPort}
			h := i.Handler.HTTPGet

			if h.Path != "" {
				a.Path = h.Path
			}

			// TODO: should error if `len(ports) == 0` and none of these are set
			if !(h.Port != 0 && h.PortName != "") {
				if h.Port != 0 {
					a.Port = intstr.FromInt(int(h.Port))
				}
				if h.PortName != "" {
					a.Port = intstr.FromString(h.PortName)
				}
			}

			if h.Host != "" {
				a.Host = h.Host
			}

			if h.Scheme != "" {
				a.Scheme = h.Scheme
			}

			if len(h.HTTPHeaders) > 0 {
				for k, v := range h.HTTPHeaders {
					a.HTTPHeaders = append(a.HTTPHeaders, v1.HTTPHeader{Name: k, Value: v})
				}
			}

			probe.Handler.HTTPGet = &a
		case 2:
			a := v1.TCPSocketAction{Port: defaultPort}
			h := i.Handler.TCPSocket

			// TODO: should error if `len(ports) == 0` and none of these are set
			if !(h.Port != 0 && h.PortName != "") {
				if h.Port != 0 {
					a.Port = intstr.FromInt(int(h.Port))
				}
				if h.PortName != "" {
					a.Port = intstr.FromString(h.PortName)
				}
			}

			probe.Handler.TCPSocket = &a
		}
	} // TODO error here

	if i.InitialDelaySeconds > 0 {
		probe.InitialDelaySeconds = i.InitialDelaySeconds
	}

	if i.TimeoutSeconds > 0 {
		probe.TimeoutSeconds = i.TimeoutSeconds
	}

	if i.PeriodSeconds > 0 {
		probe.PeriodSeconds = i.PeriodSeconds
	}

	if i.SuccessThreshold > 0 {
		probe.SuccessThreshold = i.SuccessThreshold
	}

	if i.FailureThreshold > 0 {
		probe.FailureThreshold = i.FailureThreshold
	}

	return &probe
}

func (i *Volume) Convert() v1.Volume {
	volume := v1.Volume{Name: i.Name}

	// TODO error if more then one thing is set
	whichVolumeSource := exclusiveNonNil(i.HostPath, i.EmptyDir, i.Secret)
	if whichVolumeSource != nil {
		switch *whichVolumeSource {
		case 0:
			s := v1.HostPathVolumeSource(*i.VolumeSource.HostPath)
			volume.VolumeSource.HostPath = &s
		case 1:
			s := v1.EmptyDirVolumeSource(*i.VolumeSource.EmptyDir)
			volume.VolumeSource.EmptyDir = &s
		case 2:
			s := v1.SecretVolumeSource{
				SecretName:  i.VolumeSource.Secret.SecretName,
				DefaultMode: &i.VolumeSource.Secret.DefaultMode,
				Optional:    &i.VolumeSource.Secret.Optional,
			}
			for _, item := range i.VolumeSource.Secret.Items {
				s.Items = append(s.Items, v1.KeyToPath(item))
			}
			volume.VolumeSource.Secret = &s
		}
	}

	return volume
}

func MakePod(parentMeta metav1.ObjectMeta, spec Pod) *v1.PodTemplateSpec {
	meta := metav1.ObjectMeta{
		Labels:      parentMeta.Labels,
		Annotations: spec.Annotations,
	}

	podSpec := v1.PodSpec{
		Containers: []v1.Container{},
		Volumes:    []v1.Volume{},
	}

	for _, container := range spec.Containers {
		podSpec.Containers = append(podSpec.Containers, container.Convert())
	}

	for _, volume := range spec.Volumes {
		podSpec.Volumes = append(podSpec.Volumes, volume.Convert())
	}

	pod := v1.PodTemplateSpec{
		ObjectMeta: meta,
		Spec:       podSpec,
	}

	return &pod
}

func (i *Metadata) Convert(name string) metav1.ObjectMeta {
	meta := metav1.ObjectMeta{
		Name:        name,
		Labels:      i.Labels,
		Annotations: i.Annotations,
	}

	if len(meta.Labels) == 0 {
		meta.Labels = map[string]string{"name": name}
	}

	return meta
}

func (i *Deployment) Convert() *v1beta1.Deployment {
	meta := i.Metadata.Convert(i.Name)

	pod := MakePod(meta, i.Pod)

	deploymentSpec := v1beta1.DeploymentSpec{
		Template: *pod,
		Replicas: &i.Replicas,
	}

	if len(i.Selector) == 0 {
		deploymentSpec.Selector = &metav1.LabelSelector{MatchLabels: meta.Labels}
	} else {
		deploymentSpec.Selector = &metav1.LabelSelector{MatchLabels: i.Selector}
	}

	deployment := v1beta1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "extensions/v1beta1",
		},
		ObjectMeta: meta,
		Spec:       deploymentSpec,
	}

	return &deployment
}

func (i *Service) Convert() *v1.Service {
	meta := i.Metadata.Convert(i.Name)

	serviceSpec := v1.ServiceSpec{
		Ports: []v1.ServicePort{},
	}
	if len(i.Selector) == 0 {
		serviceSpec.Selector = meta.Labels
	} else {
		serviceSpec.Selector = i.Selector
	}

	for _, port := range i.Ports {
		p := v1.ServicePort{
			Name:     port.Name,
			Port:     port.Port,
			NodePort: port.NodePort,
			// default to taget port with the same name
			TargetPort: intstr.FromString(port.Name),
		}
		if !(port.TargetPort != 0 && port.TargetPortName != "") {
			// overide the default target port
			if port.TargetPort != 0 {
				p.TargetPort = intstr.FromInt(int(port.TargetPort))
				if port.Port == 0 {
					p.Port = port.TargetPort
				}
			}
			if port.TargetPortName != "" {
				p.TargetPort = intstr.FromString(port.TargetPortName)
			}
		} // TODO: should error if both are set
		serviceSpec.Ports = append(serviceSpec.Ports, p)
	}

	service := v1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: meta,
		Spec:       serviceSpec,
	}

	return &service
}

func (i *ResourceGroup) EncodeListToPrettyJSON() ([]byte, error) {
	return util.EncodeList(i.MakeList(), "application/json", true)
}

func (i *ResourceGroup) MakeList() *api.List {
	components := &api.List{}
	for _, component := range i.Deployments {
		components.Items = append(components.Items, runtime.Object(component.Convert()))
	}
	for _, component := range i.Services {
		components.Items = append(components.Items, runtime.Object(component.Convert()))
	}
	return components
}
