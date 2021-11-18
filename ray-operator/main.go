package main

import (
	"context"
	"flag"
	"net"
	"os"

	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"github.com/ray-project/kuberay/ray-operator/controllers"
	rpc "github.com/ray-project/kuberay/ray-operator/rpc"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection" // For debugging

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	rayiov1alpha1 "github.com/ray-project/kuberay/ray-operator/api/raycluster/v1alpha1"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(rayiov1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

type server struct {
	rpc.UnimplementedNodeProviderServer
}

func (s *server) NonTerminatedNodes(ctx context.Context, in *rpc.NonTerminatedNodesRequest) (*rpc.NonTerminatedNodesResponse, error) {
	setupLog.Info("the rpc server", "received:", in.GetClusterName())
	return &rpc.NonTerminatedNodesResponse{}, nil
}

func startRpcServer() {
	setupLog.Info("starting rpc server")
	lis, err := net.Listen("tcp", ":5000")
	if err != nil {
		setupLog.Error(err, "failed to listen")
	}
	s := grpc.NewServer()
	rpc.RegisterNodeProviderServer(s, &server{})
	reflection.Register(s)
	if err := s.Serve(lis); err != nil {
		setupLog.Error(err, "failed to serve")
	}
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var reconcileConcurrency int
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.IntVar(&reconcileConcurrency, "reconcile-concurrency", 1, "max concurrency for reconciling")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	setupLog.Info("the operator", "version:", os.Getenv("OPERATOR_VERSION"))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "ray-operator-leader",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = controllers.NewReconciler(mgr).SetupWithManager(mgr, reconcileConcurrency); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RayCluster")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	go startRpcServer()

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
