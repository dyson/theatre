package integration

import (
	"context"
	"time"

	kitlog "github.com/go-kit/kit/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	workloadsv1alpha1 "github.com/gocardless/theatre/pkg/apis/workloads/v1alpha1"
	"github.com/gocardless/theatre/pkg/integration"
	"github.com/gocardless/theatre/pkg/workloads/console"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	timeout = 5 * time.Second
	logger  = kitlog.NewLogfmtLogger(GinkgoWriter)
)

var _ = Describe("Console", func() {
	var (
		ctx       context.Context
		cancel    func()
		namespace string
		teardown  func()
		mgr       manager.Manager
		calls     chan integration.ReconcileCall
		whcalls   chan integration.HandleCall
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), time.Minute)
		namespace, teardown = integration.CreateNamespace(clientset)
		mgr = integration.StartTestManager(ctx, cfg)

		integration.MustController(
			console.Add(ctx, logger, mgr,
				func(opt *controller.Options) {
					opt.Reconciler, calls = integration.CaptureReconcile(
						opt.Reconciler,
					)
				},
			),
		)

		integration.NewServer(mgr, integration.MustWebhook(
			console.NewWebhook(logger, mgr,
				func(handler *admission.Handler) {
					*handler, whcalls = integration.CaptureWebhook(mgr, *handler)
				},
			),
		))
	})

	AfterEach(func() {
		cancel()
		teardown()
	})

	Describe("Creating resources", func() {
		It("Sets console.spec.user from rbac", func() {
			csl := &workloadsv1alpha1.Console{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: namespace,
				},
				Spec: workloadsv1alpha1.ConsoleSpec{
					User: "", // deliberately not configured, this should be set by the webhook
				},
			}

			By("Creating Console 'foo'")
			Expect(mgr.GetClient().Create(context.TODO(), csl)).NotTo(
				HaveOccurred(), "failed to create 'foo' Console",
			)

			By("Expect webhook was invoked")
			Eventually(whcalls, timeout).Should(
				Receive(
					integration.HandleResource(namespace, "foo"),
				),
			)

			By("Expect reconcile succeeded")
			Eventually(calls, timeout).Should(
				Receive(
					integration.ReconcileResourceSuccess(namespace, "foo"),
				),
			)

			By("Expect console.spec.user to be set")
			Expect(csl.Spec.User).To(Equal("system:unsecured"))
		})
	})
})
