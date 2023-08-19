package main

import (
	"fmt"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ecr"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ecs"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/lb"
	ecsx "github.com/pulumi/pulumi-awsx/sdk/go/awsx/ecs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	type Data struct {
		SleepTime string `yaml:"sleepTime"`
		VPCID     string `yaml:"vpcID"`
	}

	pulumi.Run(func(ctx *pulumi.Context) error {
		env := ctx.Stack()
		app := "scribe-backend"

		var d Data
		conf := config.New(ctx, "")
		conf.RequireObject("config", &d)

		tags := pulumi.StringMap{
			"Env":       pulumi.String(env),
			"Source":    pulumi.String("pulumi"),
			"Project":   pulumi.String(app),
			"Directory": pulumi.String("infrastructure/pulumi"),
		}

		image, err := ecr.GetImage(ctx, &ecr.GetImageArgs{
			ImageTag:       &env,
			RepositoryName: "scribe-backend",
		})
		if err != nil {
			return fmt.Errorf("error getting image from ecr: %w", err)
		}

		cluster, err := ecs.LookupCluster(ctx, &ecs.LookupClusterArgs{
			ClusterName: fmt.Sprintf("%s-fargate", env),
		}, nil)
		if err != nil {
			return fmt.Errorf("error lookup cluster: %w", err)
		}

		vpc, err := ec2.GetVpc(ctx, "main", pulumi.ID(d.VPCID), nil, nil)
		if err != nil {
			return fmt.Errorf("error getting the VPC: %w", err)
		}

		securityGroupALB, err := createAlbSG(ctx, vpc, app, env, tags)
		if err != nil {
			return fmt.Errorf("error creating alb sg: %w", err)
		}
		securityGroupECS, err := createEcsSG(ctx, vpc, app, env, tags)
		if err != nil {
			return fmt.Errorf("error creating ecs sg: %w", err)
		}

		snPrivate, err := ec2.GetSubnets(ctx, &ec2.GetSubnetsArgs{
			Filters: []ec2.GetSubnetsFilter{{
				Name:   "tag:Name",
				Values: []string{fmt.Sprintf("%s-vpc-private*", env)},
			}},
		}, nil)

		snPublic, err := ec2.GetSubnets(ctx, &ec2.GetSubnetsArgs{
			Filters: []ec2.GetSubnetsFilter{{
				Name:   "tag:Name",
				Values: []string{fmt.Sprintf("%s-vpc-public*", env)},
			}},
		}, nil)
		if err != nil {
			return fmt.Errorf("error getting public subnets: %w", err)
		}

		albTG, err := lb.NewTargetGroup(ctx, fmt.Sprintf("%s-%s-tg", env, app), &lb.TargetGroupArgs{
			HealthCheck: lb.TargetGroupHealthCheckArgs{
				Path: pulumi.StringPtr("/health"),
				Port: pulumi.Sprintf("%d", 8080),
			},
			Name:       pulumi.StringPtr(fmt.Sprintf("%s-%s-tg", env, app)),
			Port:       pulumi.IntPtr(8080),
			Protocol:   pulumi.StringPtr("HTTP"),
			Tags:       tags,
			TargetType: pulumi.StringPtr("ip"),
			VpcId:      pulumi.StringPtr(d.VPCID),
		}, nil)
		if err != nil {
			return fmt.Errorf("error creating target group: %w", err)
		}

		_, err = createAlb(ctx, albTG, securityGroupALB, snPublic, app, env, tags)
		if err != nil {
			return fmt.Errorf("error creating alb: %w", err)
		}

		//alb, err := lbx.NewApplicationLoadBalancer(ctx, fmt.Sprintf("%s-%s-lb", env, app), &lbx.ApplicationLoadBalancerArgs{
		//	DefaultTargetGroupPort: pulumi.IntPtr(80),
		//	Listeners: []lbx.ListenerArgs{
		//		{
		//			CertificateArn: nil,
		//			DefaultActions: lb.ListenerDefaultActionArray{lb.ListenerDefaultActionArgs{
		//				TargetGroupArn: albTG.Arn,
		//				Type:           pulumi.Sprintf("forward"),
		//			}},
		//			Port:     pulumi.IntPtr(80),
		//			Protocol: pulumi.StringPtr("HTTP"),
		//			Tags:     tags,
		//		},
		//	},
		//	Name:                    pulumi.String(fmt.Sprintf("%s-%s-lb", env, app)),
		//	NamePrefix:              nil,
		//	PreserveHostHeader:      nil,
		//	SecurityGroups:          nil,
		//	SubnetIds:               pulumi.ToStringArray(snPublic.Ids),
		//	SubnetMappings:          nil,
		//	Subnets:                 nil,
		//	Tags:                    tags,
		//	XffHeaderProcessingMode: nil,
		//})
		//if err != nil {
		//	return fmt.Errorf("error creating alb: %w", err)
		//}

		_, err = ecsx.NewFargateService(ctx, app, &ecsx.FargateServiceArgs{
			Name:    pulumi.StringPtr(app),
			Cluster: pulumi.StringPtr(cluster.Arn),
			NetworkConfiguration: &ecs.ServiceNetworkConfigurationArgs{
				Subnets: pulumi.ToStringArray(snPrivate.Ids),
				SecurityGroups: pulumi.StringArray{
					securityGroupECS.ID(),
				},
				AssignPublicIp: pulumi.BoolPtr(false),
			},
			DesiredCount: pulumi.Int(1),
			LoadBalancers: ecs.ServiceLoadBalancerArray{
				&ecs.ServiceLoadBalancerArgs{
					ContainerName:  pulumi.String(app),
					ContainerPort:  pulumi.Int(8080),
					TargetGroupArn: albTG.Arn,
				},
			},
			TaskDefinitionArgs: &ecsx.FargateServiceTaskDefinitionArgs{
				Container: &ecsx.TaskDefinitionContainerDefinitionArgs{
					Name:      pulumi.StringPtr(app),
					Cpu:       pulumi.Int(512),
					Essential: pulumi.Bool(true),
					Image:     pulumi.Sprintf("657548505037.dkr.ecr.us-west-2.amazonaws.com/%s:%s@%s", app, env, image.ImageDigest),
					Memory:    pulumi.Int(128),
					PortMappings: ecsx.TaskDefinitionPortMappingArray{
						&ecsx.TaskDefinitionPortMappingArgs{
							ContainerPort: pulumi.Int(8080),
							HostPort:      pulumi.Int(8080),
							Protocol:      pulumi.String("tcp"),
						},
					},
				},
				Tags: tags,
			},
			Tags: tags,
		})

		// Deploy an ECS Service on Fargate to host the application container
		//_, err = ecsx.NewFargateService(ctx, "service", &ecsx.FargateServiceArgs{
		//	Cluster:        pulumi.StringPtr(cluster.Arn),
		//	AssignPublicIp: pulumi.Bool(true),
		//	TaskDefinitionArgs: &ecsx.FargateServiceTaskDefinitionArgs{
		//		Container: &ecsx.TaskDefinitionContainerDefinitionArgs{
		//			Image:     pulumi.StringPtr(img.ImageDigest),
		//			Cpu:       pulumi.Int(512),
		//			Memory:    pulumi.Int(128),
		//			Essential: pulumi.Bool(true),
		//			PortMappings: ecsx.TaskDefinitionPortMappingArray{
		//				&ecsx.TaskDefinitionPortMappingArgs{
		//					ContainerPort: pulumi.Int(80),
		//					TargetGroup:   loadbalancer.DefaultTargetGroup,
		//				},
		//			},
		//		},
		//	},
		//	Tags: pulumi.StringMap{
		//		"Env":       pulumi.String(env),
		//		"Source":    pulumi.String("pulumi"),
		//		"Project":   pulumi.String("scribe-backend"),
		//		"Directory": pulumi.String("infrastructure/pulumi"),
		//	},
		//})
		if err != nil {
			return fmt.Errorf("error creating fargate service: %w", err)
		}

		return nil
	})
}

func createAlb(ctx *pulumi.Context, albTG *lb.TargetGroup, sgALB *ec2.SecurityGroup, snPublic *ec2.GetSubnetsResult, app string, env string, tags pulumi.StringMap) (*lb.LoadBalancer, error) {
	alb, err := lb.NewLoadBalancer(ctx, fmt.Sprintf("%s-%s-alb", env, app), &lb.LoadBalancerArgs{
		Internal:         pulumi.BoolPtr(false),
		LoadBalancerType: pulumi.StringPtr("application"),
		Name:             pulumi.Sprintf("%s-%s-alb", env, app),
		SecurityGroups:   pulumi.StringArray{sgALB.ID()},
		Subnets:          pulumi.ToStringArray(snPublic.Ids),
		Tags:             tags,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating new load balancer: %w", err)
	}

	_, err = lb.NewListener(ctx, fmt.Sprintf("%s-%s-80-listener", app, env), &lb.ListenerArgs{
		DefaultActions: lb.ListenerDefaultActionArray{lb.ListenerDefaultActionArgs{
			TargetGroupArn: albTG.Arn,
			Type:           pulumi.Sprintf("forward"),
		}},
		LoadBalancerArn: alb.Arn,
		Port:            pulumi.IntPtr(80),
		Protocol:        pulumi.StringPtr("HTTP"),
		Tags:            tags,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating new listener in port 80: %w", err)
	}

	//_, err = lb.NewListener(ctx, fmt.Sprintf("%s-%s-443-listener", app, env), &lb.ListenerArgs{
	//	DefaultActions: lb.ListenerDefaultActionArray{lb.ListenerDefaultActionArgs{
	//		TargetGroupArn: albTG.Arn,
	//		Type:           pulumi.String("forward"),
	//	}},
	//	LoadBalancerArn: alb.Arn,
	//	Port:            pulumi.IntPtr(443),
	//	Protocol:        pulumi.StringPtr("HTTPS"),
	//	SslPolicy:       pulumi.StringPtr("ELBSecurityPolicy-2016-08"),
	//	Tags:            tags,
	//}, nil)
	//if err != nil {
	//	return nil, fmt.Errorf("error creating new listener in port 443: %w", err)
	//}
	//
	return alb, nil
}

func createAlbSG(ctx *pulumi.Context, vpc *ec2.Vpc, app string, env string, tags pulumi.StringMap) (*ec2.SecurityGroup, error) {
	return ec2.NewSecurityGroup(ctx, fmt.Sprintf("%s-%s-alb-sg", app, env), &ec2.SecurityGroupArgs{
		Description:         pulumi.StringPtr("Pulumi Managed"),
		Name:                pulumi.Sprintf("%s-%s-alb-sg", app, env),
		VpcId:               vpc.ID(),
		RevokeRulesOnDelete: pulumi.BoolPtr(true),
		Tags:                tags,
		Egress: ec2.SecurityGroupEgressArray{
			&ec2.SecurityGroupEgressArgs{
				FromPort:       pulumi.Int(0),
				Protocol:       pulumi.String("-1"),
				ToPort:         pulumi.Int(0),
				CidrBlocks:     pulumi.StringArray{pulumi.String("0.0.0.0/0")},
				Ipv6CidrBlocks: pulumi.StringArray{pulumi.String("::/0")},
			},
		},
		Ingress: ec2.SecurityGroupIngressArray{
			&ec2.SecurityGroupIngressArgs{
				CidrBlocks:     pulumi.StringArray{vpc.CidrBlock, pulumi.String("0.0.0.0/0")},
				Ipv6CidrBlocks: pulumi.StringArray{pulumi.String("::/0")},
				FromPort:       pulumi.Int(80),
				Protocol:       pulumi.String("tcp"),
				ToPort:         pulumi.Int(80),
			},
			//&ec2.SecurityGroupIngressArgs{
			//	CidrBlocks:     pulumi.StringArray{vpc.CidrBlock, pulumi.String("0.0.0.0/0")},
			//	FromPort:       pulumi.Int(443),
			//	Ipv6CidrBlocks: pulumi.StringArray{pulumi.String("::/0")},
			//	Protocol:       pulumi.String("tcp"),
			//	ToPort:         pulumi.Int(443),
			//},
			&ec2.SecurityGroupIngressArgs{
				FromPort: pulumi.Int(3004),
				Protocol: pulumi.String("tcp"),
				ToPort:   pulumi.Int(3004),
			},
		},
	}, nil)
}
func createEcsSG(ctx *pulumi.Context, vpc *ec2.Vpc, app string, env string, tags pulumi.StringMap) (*ec2.SecurityGroup, error) {
	return ec2.NewSecurityGroup(ctx, fmt.Sprintf("%s-%s-ecs-sg", app, env), &ec2.SecurityGroupArgs{
		Description:         pulumi.StringPtr("Pulumi Managed"),
		Name:                pulumi.Sprintf("%s-%s-ecs-sg", app, env),
		VpcId:               vpc.ID(),
		RevokeRulesOnDelete: pulumi.BoolPtr(true),
		Tags:                tags,
		Egress: ec2.SecurityGroupEgressArray{
			&ec2.SecurityGroupEgressArgs{
				FromPort:       pulumi.Int(0),
				Protocol:       pulumi.String("-1"),
				ToPort:         pulumi.Int(0),
				CidrBlocks:     pulumi.StringArray{pulumi.String("0.0.0.0/0")},
				Ipv6CidrBlocks: pulumi.StringArray{pulumi.String("::/0")},
			},
		},
		Ingress: ec2.SecurityGroupIngressArray{
			&ec2.SecurityGroupIngressArgs{
				FromPort: pulumi.Int(8080),
				Protocol: pulumi.String("tcp"),
				ToPort:   pulumi.Int(8080),
			},
		},
	}, nil)
}
