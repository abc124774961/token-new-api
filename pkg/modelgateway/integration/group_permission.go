package integration

import "github.com/QuantumNous/new-api/service"

type ServiceGroupPermissionService struct{}

func NewServiceGroupPermissionService() *ServiceGroupPermissionService {
	return &ServiceGroupPermissionService{}
}

func (s *ServiceGroupPermissionService) GetUserUsableGroups(userGroup string) map[string]string {
	return service.GetUserUsableGroups(userGroup)
}

func (s *ServiceGroupPermissionService) GroupInUserUsableGroups(userGroup, groupName string) bool {
	return service.GroupInUserUsableGroups(userGroup, groupName)
}

func (s *ServiceGroupPermissionService) GetUserAutoGroup(userGroup string) []string {
	return service.GetUserAutoGroup(userGroup)
}

func (s *ServiceGroupPermissionService) EffectiveRoutingGroups(groupName string) []string {
	return service.EffectiveRoutingGroups(groupName)
}
