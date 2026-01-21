export interface MarketplaceAddon {
  _id?: string;
  name: string;
  description?: string;
  services: AddonService[];
  volumes?: AddonVolume[];
  networks?: AddonNetwork[];
}

export interface AddonService {
  service_name: string;
  image: string;
  command?: string;
  ports?: { [key: string]: string };
  environment?: { [key: string]: string };
  volumes?: string[];
  labels?: { [key: string]: string };
  networks?: string[];
}

export interface AddonVolume {
  name: string;
  driver?: string;
  labels?: { [key: string]: string };
  driver_opts?: { [key: string]: string };
}

export interface AddonNetwork {
  name: string;
  enable_ipv6?: boolean;
  driver?: string;
}

export interface InstalledAddon {
  _id: string;
  marketplace_id: string;
  name?: string;
  description?: string;
  status: AddonStatus;
  created_at?: string;
  updated_at?: string;
}

export enum AddonStatus {
  INSTALLING = 'INSTALLING',
  RUNNING = 'RUNNING',
  FAILED = 'FAILED',
  DISABLING = 'DISABLING'
}

export interface Hook {
  _id?: string;
  hook_name: string;
  webhook_url: string;
  entity: string;
  events: HookEvent[];
}

export enum HookEvent {
  POST_CREATE = 'post_create',
  PRE_CREATE = 'pre_create',
  POST_UPDATE = 'post_update',
  PRE_UPDATE = 'pre_update',
  POST_DELETE = 'post_delete',
  PRE_DELETE = 'pre_delete'
}

export interface CustomResource {
  _id?: string;
  resource_type: string;
  schema?: any;
}

export interface AppConfig {
  marketplaceUrl: string;
  addonsEngineUrl: string;
  resourceAbstractorUrl: string;
}
