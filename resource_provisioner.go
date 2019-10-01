package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hashicorp/terraform/communicator"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/terraform"
	"github.com/martezr/terraform-provisioner-puppet-bolt/bolt"
)

type provisionFn func(terraform.UIOutput, communicator.Communicator) error

type provisioner struct {
	BoltTimeout time.Duration
	ModulePath  string
	Parameters  map[string]interface{}
	Password    string
	Plan        string
	Task        string
	Username    string
	UseSudo     bool

	instanceState *terraform.InstanceState
	output        terraform.UIOutput
	comm          communicator.Communicator
}

func Provisioner() terraform.ResourceProvisioner {
	return &schema.Provisioner{
		Schema: map[string]*schema.Schema{
			"username": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},

			"password": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},
			"task": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},
			"plan": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},
			"parameters": &schema.Schema{
				Type:     schema.TypeMap,
				Optional: true,
			},
			"module_path": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},
			"use_sudo": &schema.Schema{
				Type:     schema.TypeBool,
				Optional: true,
				Default:  true,
			},
			"bolt_timeout": &schema.Schema{
				Type:     schema.TypeString,
				Default:  "5m",
				Optional: true,
				ValidateFunc: func(val interface{}, key string) (warns []string, errs []error) {
					_, err := time.ParseDuration(val.(string))
					if err != nil {
						errs = append(errs, err)
					}
					return warns, errs
				},
			},
		},
		ApplyFunc: applyFn,
	}
}

func applyFn(ctx context.Context) error {
	output := ctx.Value(schema.ProvOutputKey).(terraform.UIOutput)
	state := ctx.Value(schema.ProvRawStateKey).(*terraform.InstanceState)
	configData := ctx.Value(schema.ProvConfigDataKey).(*schema.ResourceData)

	p, err := decodeConfig(configData)
	if err != nil {
		return err
	}

	p.instanceState = state
	p.output = output

	comm, err := communicator.New(state)
	if err != nil {
		return err
	}

	retryCtx, cancel := context.WithTimeout(ctx, comm.Timeout())
	defer cancel()

	err = communicator.Retry(retryCtx, func() error {
		return comm.Connect(output)
	})
	if err != nil {
		return err
	}
	defer comm.Disconnect()

	output.Output("Running Puppet Bolt Task")
	output.Output(fmt.Sprintf("Bolt Parameters: %s", p.Parameters))
	paramJSON, err := json.Marshal(p.Parameters)
	//if err != nil {
	//	output.Output(fmt.Sprintf(err.Error()))
	//}
	jsonStr := string(paramJSON)
	output.Output(fmt.Sprintf("Bolt Parameters JSON: %s", jsonStr))
	result, err := bolt.Plan(
		p.instanceState.Ephemeral.ConnInfo,
		p.BoltTimeout,
		p.UseSudo,
		p.Plan,
		p.Parameters,
		p.ModulePath,
		nil,
	)
	output.Output(fmt.Sprintf("puppet_agent::install failed: %s\n%+v", err, result))
	/*
			result, err := bolt.Task(
				p.instanceState.Ephemeral.ConnInfo,
				p.BoltTimeout,
				p.UseSudo,
				p.Task,
				nil,
			)

		if err != nil || result.Items[0].Status != "success" {
			output.Output(fmt.Sprintf("puppet_agent::install failed: %s\n%+v", err, result))
		} else {
			output.Output(fmt.Sprintf("Puppet Bolt task results: %+v", result))
		}
	*/
	return nil
}

func decodeConfig(d *schema.ResourceData) (*provisioner, error) {
	p := &provisioner{
		UseSudo:    d.Get("use_sudo").(bool),
		Password:   d.Get("password").(string),
		Username:   d.Get("username").(string),
		Task:       d.Get("task").(string),
		Plan:       d.Get("plan").(string),
		Parameters: d.Get("parameters").(map[string]interface{}),
		ModulePath: d.Get("module_path").(string),
	}
	p.BoltTimeout, _ = time.ParseDuration(d.Get("bolt_timeout").(string))

	return p, nil
}
