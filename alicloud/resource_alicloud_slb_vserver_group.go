package alicloud

import (
	"fmt"
	"github.com/denverdino/aliyungo/slb"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	"time"
	"encoding/json"
	"bytes"
	"strings"
	"github.com/hashicorp/terraform/helper/hashcode"
)

func resourceAliyunSlbVServerGroup() *schema.Resource {
	return &schema.Resource{
		Create: resourceAliyunSlbVServerGroupCreate,
		Read:   resourceAliyunSlbVServerGroupRead,
		Update: resourceAliyunSlbVServerGroupUpdate,
		Delete: resourceAliyunSlbVServerGroupDelete,

		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:         schema.TypeString,
				Required: true,
				//ValidateFunc: validateSlbVServerGroupName,
			},

			"slb_id": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},

			"instances": &schema.Schema{
				Type:     schema.TypeSet,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"server_id": &schema.Schema{
							Type:         schema.TypeString,
							Required:     true,
						},

						"port": &schema.Schema{
							Type:         schema.TypeInt,
							ValidateFunc: validateInstancePort,
							Required:     true,
						},

						"weight": &schema.Schema{
							Type:         schema.TypeInt,
							Required:     true,
						},
					},
				},
				Set: resourceAliyunSlbVServerGroupInstancesHash,
			},
		},
	}
}

func resourceAliyunSlbVServerGroupCreate(d *schema.ResourceData, meta interface{}) error {
	slbconn := meta.(*AliyunClient).slbconn

	vServerGroupArgs := &slb.CreateVServerGroupArgs{
		RegionId:         getRegion(d, meta),
	}

	if v := d.Get("name").(string); v != "" {
		vServerGroupArgs.VServerGroupName = v
	}

	if v := d.Get("slb_id").(string); v != "" {
		vServerGroupArgs.LoadBalancerId = v
	}

	if d.HasChange("instances") {
		o, n := d.GetChange("instances")
		os := o.(*schema.Set)
		ns := n.(*schema.Set)
		add := expandVBackendServers(ns.Difference(os).List())

		if len(add) > 0 {
			bytes, _ := json.Marshal(add)
			vServerGroupArgs.BackendServers = string(bytes)
			vServerGroup, err := slbconn.CreateVServerGroup(vServerGroupArgs)
			if err != nil {
				return err
			}
			d.SetId(vServerGroup.VServerGroupId)
		}
	}

	return resourceAliyunSlbVServerGroupRead(d, meta)
}

func resourceAliyunSlbVServerGroupRead(d *schema.ResourceData, meta interface{}) error {
	slbconn := meta.(*AliyunClient).slbconn

	describeVServerGroupAttributeArgs := &slb.DescribeVServerGroupAttributeArgs{
		RegionId:         getRegion(d, meta),
		VServerGroupId:   d.Id(),
	}
	vServerGroup, err := slbconn.DescribeVServerGroupAttribute(describeVServerGroupAttributeArgs)
	if err != nil {
		if notFoundError(err) {
			d.SetId("")
			return nil
		}

		return err
	}

	if vServerGroup == nil {
		d.SetId("")
		return nil
	}

	d.Set("name", vServerGroup.VServerGroupName)

	return nil
}

func resourceAliyunSlbVServerGroupUpdate(d *schema.ResourceData, meta interface{}) error {
	slbconn := meta.(*AliyunClient).slbconn

	d.Partial(true)

	if d.HasChange("instances") {
		o, n := d.GetChange("instances")
		os := o.(*schema.Set)
		ns := n.(*schema.Set)
		remove := expandVBackendServers(os.Difference(ns).List())
		add := expandVBackendServers(ns.Difference(os).List())

		if len(add) > 0 {
			bytes, _ := json.Marshal(add)
			addVServerGroupBackendServerArgs := &slb.AddVServerGroupBackendServersArgs{
				RegionId:       getRegion(d, meta),
				VServerGroupId: d.Id(),
				BackendServers: string(bytes),
			}
			_, err := slbconn.AddVServerGroupBackendServers(addVServerGroupBackendServerArgs)
			if err != nil {
				return err
			}
		}
		if len(remove) > 0 {
			bytes, _ := json.Marshal(remove)
			removeVServerGroupBackendServerArgs := &slb.RemoveVServerGroupBackendServersArgs{
				RegionId:         getRegion(d, meta),
				VServerGroupId: d.Id(),
				BackendServers:   string(bytes),
			}
			_, err := slbconn.RemoveVServerGroupBackendServers(removeVServerGroupBackendServerArgs)
			if err != nil {
				return err
			}
		}

		d.SetPartial("instances")
	}

	d.Partial(false)

	return resourceAliyunSlbVServerGroupRead(d, meta)
}

func resourceAliyunSlbVServerGroupDelete(d *schema.ResourceData, meta interface{}) error {
	slbconn := meta.(*AliyunClient).slbconn

	deleteVServerGroupArgs := &slb.DeleteVServerGroupArgs{
		RegionId:         getRegion(d, meta),
		VServerGroupId:   d.Id(),
	}

	describeVServerGroupAttributeArgs := &slb.DescribeVServerGroupAttributeArgs{
		RegionId:         getRegion(d, meta),
		VServerGroupId:   d.Id(),
	}

	return resource.Retry(5*time.Minute, func() *resource.RetryError {
		_, err := slbconn.DeleteVServerGroup(deleteVServerGroupArgs)

		if err != nil {
			return resource.RetryableError(fmt.Errorf("VServerGroup in use - trying again while it is deleted."))
		}

		vServerGroup, err := slbconn.DescribeVServerGroupAttribute(describeVServerGroupAttributeArgs)
		if vServerGroup != nil {
			return resource.RetryableError(fmt.Errorf("VServerGroup in use - trying again while it is deleted."))
		}
		return nil
	})
}

func resourceAliyunSlbVServerGroupInstancesHash(v interface{}) int {
	var buf bytes.Buffer
	m := v.(map[string]interface{})
	buf.WriteString(fmt.Sprintf("%s-",
		strings.ToLower(m["server_id"].(string))))
	buf.WriteString(fmt.Sprintf("%d-", m["port"].(int)))
	buf.WriteString(fmt.Sprintf("%d-", m["weight"].(int)))

	return hashcode.String(buf.String())
}
