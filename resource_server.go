package main

import (
	"fmt"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/scaleway/scaleway-cli/pkg/api"
)

func resourceServer() *schema.Resource {
	return &schema.Resource{
		Create: resourceServerCreate,
		Read:   resourceServerRead,
		Update: resourceServerUpdate,
		Delete: resourceServerDelete,
		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			"image": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"volumes": &schema.Schema{
				Type:     schema.TypeMap,
				Required: true,
			},
			"dynamic_ip_required": &schema.Schema{
				Type:     schema.TypeBool,
				Optional: true,
			},
			"bootscript": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},
			"tags": &schema.Schema{
				Type: schema.TypeList,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Optional: true,
			},
			"ipv4_address": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
			"ipv4_address_private": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
			"state": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
			"state_detail": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}

}

func resourceServerCreate(d *schema.ResourceData, m interface{}) error {
	scaleway := m.(*api.ScalewayAPI)
	d.Partial(true)

	image := d.Get("image").(string)

	volumes := make(map[string]string)

	for k, v := range d.Get("volumes").(map[string]interface{}) {
		volumes[k] = v.(string)
	}

	var def api.ScalewayServerDefinition

	def.Name = d.Get("name").(string)
	def.Image = &image
	def.Volumes = volumes

	if dynamicIPRequiredI, ok := d.GetOk("dynamic_ip_required"); ok {
		dynamicIPRequired := dynamicIPRequiredI.(bool)
		def.DynamicIPRequired = &dynamicIPRequired
	}

	if bootscriptI, ok := d.GetOk("bootscript"); ok {
		bootscript := bootscriptI.(string)
		def.Bootscript = &bootscript
	}

	if tags, ok := d.GetOk("tags"); ok {
		def.Tags = tags.([]string)
	}

	id, err := scaleway.PostServer(def)
	if err != nil {
		return fmt.Errorf("Error Posting server with image %s", image)
	}

	err = scaleway.PostServerAction(id, "poweron")

	if err != nil {
		return err
	}

	d.SetId(id)

	// maybe set Partial

	_, err = api.WaitForServerReady(scaleway, id, "")

	if err != nil {
		return err
	}

	// FIXME: I'm not sure if this is effective.
	// but we had times where the    scaleway.GetServer failed
	// causing an "untainted" state without an ip address.
	// this screws up stuff like ansible
	d.SetPartial("id")

	err = resourceServerRead(d, m)
	if err != nil {
		return err
	}
	d.SetPartial("ipv4_address")
	d.Partial(false)
	return nil
}

func resourceServerRead(d *schema.ResourceData, m interface{}) error {
	scaleway := m.(*api.ScalewayAPI)
	server, err := scaleway.GetServer(d.Id())

	if err != nil {
		// TODO: make sure it's ScalewayAPIError or it might crash
		serr := err.(api.ScalewayAPIError)

		// if the resource was destroyed, destroy the resource locally
		if serr.StatusCode == 404 {
			d.SetId("")
			return nil
		}
		return err
	}

	// S.t. it's compactible with terraform-ansible
	d.Set("ipv4_address", server.PublicAddress.IP)
	d.Set("ipv4_address_private", server.PrivateIP)
	d.Set("state", server.State)
	d.Set("state_detail", server.StateDetail)

	d.SetConnInfo(map[string]string{
		"type": "ssh",
		"host": server.PublicAddress.IP,
	})

	// TODO: set more fields
	return nil
}

func resourceServerUpdate(d *schema.ResourceData, m interface{}) error {
	scaleway := m.(*api.ScalewayAPI)

	var server api.ScalewayServerPatchDefinition

	if d.HasChange("name") {
		name := d.Get("name").(string)
		server.Name = &name
	}

	if d.HasChange("volumes") {
		volumes := make(map[string]api.ScalewayVolume)
		for k, v := range d.Get("volumes").(map[string]interface{}) {
			volumes[k] = api.ScalewayVolume{
				Identifier: v.(string),
			}
		}
		server.Volumes = &volumes
	}

	scaleway.PatchServer(d.Id(), server)

	return nil
}

func resourceServerDelete(d *schema.ResourceData, m interface{}) error {
	scaleway := m.(*api.ScalewayAPI)

	// TODO: does this delete associated volumes???
	err := scaleway.DeleteServerSafe(d.Id())
	if err != nil {
		return err
	}
	return nil
}
