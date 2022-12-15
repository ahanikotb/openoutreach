import axios, { AxiosInstance } from "axios";
import Papa from "papaparse";

const BACKENDLINK = "http://localhost:8080/";
export enum TimeUnit {
  SECOND = "SECOND",
  MINUTE = "MINUTE",
  HOUR = "HOUR",
  DAY = "DAY",
  WEEK = "WEEK",
  YEAR = "YEAR",
  MONTH = "MONTH",
}
export type ParseUploadedLeadsOptions = {
  FirstNameField: string;
  LastNameField: string;
  PersonalizedLineField: string;
  EmailField: string;
};
export default class Upreach {
  client: AxiosInstance;
  jwtToken: string;
  static utils = {
    formatEmailForViewing(s: string): string {
      let r = s;
      //@ts-ignore
      r = s.replaceAll("<br/>", "\n");
      return r;
    },
    formatEmailForSending(s: string): string {
      let r = s;
      //@ts-ignore
      r = s.replaceAll("\n", "<br/>");
      return r;
    },
    reverseCalculateOffsetTime(timeInSeconds: number): {
      multiplier: number;
      timeUnit: TimeUnit;
    } {
      let time = timeInSeconds / Time.HOUR;

      if (timeInSeconds / Time.YEAR > 0 && timeInSeconds % Time.YEAR == 0) {
        time = timeInSeconds / Time.YEAR;
        return { multiplier: time, timeUnit: TimeUnit.YEAR };
      }
      if (timeInSeconds / Time.MONTH > 0 && timeInSeconds % Time.MONTH == 0) {
        time = timeInSeconds / Time.MONTH;
        return { multiplier: time, timeUnit: TimeUnit.MONTH };
      }

      if (timeInSeconds / Time.WEEK > 0 && timeInSeconds % Time.WEEK == 0) {
        time = timeInSeconds / Time.WEEK;
        return { multiplier: time, timeUnit: TimeUnit.WEEK };
      }
      if (timeInSeconds / Time.DAY > 0 && timeInSeconds % Time.DAY == 0) {
        time = timeInSeconds / Time.DAY;
        return { multiplier: time, timeUnit: TimeUnit.DAY };
      }
      return { multiplier: time, timeUnit: TimeUnit.HOUR };
    },

    parseUploadedLeads(csv: string, opts: ParseUploadedLeadsOptions): Lead[] {
      const leads: Lead[] = [];

      Papa.parse(csv, {
        complete: function (results: any) {
          let fnameIndex = 0;
          let emailIndex = 0;
          let LastNameIndex = 0;
          let PersonalizedLineIndex = 0;

          //@ts-ignore
          results.data[0].find((v, i) => {
            if (v == opts.EmailField) {
              emailIndex = i;
            }
            if (v == opts.FirstNameField) {
              fnameIndex = i;
            }
            if (v == opts.LastNameField) {
              LastNameIndex = i;
            }
            if (v == opts.PersonalizedLineField) {
              PersonalizedLineIndex = i;
            }
          });
          for (let i in results.data) {
            if (i == "0") {
              continue;
            }

            leads.push({
              CampaignID: 0,
              FirstName: results.data[i][fnameIndex],
              LastName: results.data[i][LastNameIndex],
              PersonalizedLine: results.data[i][PersonalizedLineIndex],
              Email: results.data[i][emailIndex],
            });
          }
        },
      });
      return leads;
    },
    secondsUntilDate(date: Date): number {
      console.log(date);
      var now = new Date();
      var dif = date.getTime() - now.getTime();
      if (dif <= 0) {
        return 0;
      }
      return Math.round(Math.abs(date.getTime() - now.getTime()) / 1000);
    },
    calculateOffsetTime(multiplier: number, unit: TimeUnit): number {
      return Time[unit] * multiplier;
    },
  };

  static async signUp(
    email: string,
    password: string
  ): Promise<{ token: string; user: User }> {
    let res = await axios.postForm(BACKENDLINK + "api" + "/user/signup", {
      email,
      password,
    });

    return { token: res.data.token, user: res.data.user };
  }

  static async signIn(
    email: string,
    password: string
  ): Promise<{ token: string; user: User }> {
    let res = await axios.postForm(BACKENDLINK + "api" + "/user/signin", {
      email,
      password,
    });
    return { token: res.data.token, user: res.data.user };
  }
  constructor(apiKey: string) {
    this.jwtToken = apiKey;
    this.client = axios.create({
      baseURL: BACKENDLINK + "api",
      timeout: 1000,
      headers: {
        "X-API-KEY": apiKey,
      },
    });
  }
  async getUserData(): Promise<User> {
    let res = await this.client.get(BACKENDLINK + "api" + "/user/getuser");
    return res.data.user;
  }
  async changeUserSettings(settings: UserSettingsReq) {
    let res = await this.client.post(
      BACKENDLINK + "api" + "/user/settings/update",
      settings
    );
  }
  async getCampaign(
    id: number,
    { detailed }: GetCampaignOptions
  ): Promise<Campaign> {
    let link = `/campaign/${id}/get_campaign`;
    if (detailed) {
      link = link + "/detailed";
    }
    let res = await this.client.get(link);
    for (let email of res.data.campaign.EmailSequence.Emails) {
      if (email.TimeOffset != 0) {
        email.TimeOffset = email.TimeOffset / 1000000000;
      }
    }

    return res.data.campaign;
  }
  async startCampaign(id: number, req?: StartCampaignOptions) {
    let link = `/campaign/${id}/start`;
    let res = await this.client.post(link, req);
  }
  async stopCampaign(id: number) {
    let link = `/campaign/${id}/stop`;
    let res = await this.client.get(link);
  }
  async getGmailConnectLink() {
    let link = `/user/connect/gmail`;
    let res = await this.client.get(link);
    return res.data.authUrl;
  }
  async getCampaigns(options?: GetCampaignOptions): Promise<Campaign[]> {
    let link = "/campaign/get_campaigns";
    if (options) {
      if (options.detailed) {
        link = link + "/detailed";
      }
    }
    let res = await this.client.get(link);
    return res.data.campaigns;
  }

  async createCampaign(c_req: CampaignRequest): Promise<Campaign[]> {
    let res = await this.client.post("/campaign/create", c_req);
    return res.data.campaigns;
  }
  async addLeadsToCampaign(lead_req: LeadReq) {
    let res = await this.client.post("/campaign/add_leads", lead_req);
  }

  async addEmailSeqToCampaign(email_seq_req: EmailSeqReq) {
    let res = await this.client.post("/campaign/add_email_seq", email_seq_req);
  }
  async getCampaignStats(id: number): Promise<Stats> {
    let res = await this.client.get(`/campaign/${id}/stats`);
    return res.data.stats;
  }
}

export type CampaignRequest = {
  Name: string;
};

export type User = {
  Email: string;
  Password: string;
  Settings: any;
  Campaings: Campaign[];
  UPREACHLABELID: string;
  UPREACHLABELIDOUT: string;
  GmailActivated: boolean;
  HISTORYID: number;
  LASTSYNCTIME: number;
};

export type Campaign = {
  ID: number;
  TaskCampaignID: number;
  EmailSequence: EmailSequence;
  Leads: Lead[];
  TimeStarted: string;
  Status: string;
  UserID: number;
  Stats: Stats;
  Name: string;
};

export type Stats = {
  Opens: number;
  SequenceLength: number;
  LeadsAmount: number;
  OpenRate: number;
  EmailsSent: number;
  StepStats: StepStat[];
  ReplyRate: number;
  Replies: number;
  LinkClicks: number;
  ClickThroughRate: number;
};
export type StepStat = {
  ID: number;
  OpenedEmails: number;
  OpenRate: number;
  SentEmails: number;
  ReplyRate: number;
  Replies: number;
  LinkClicks: number;
  ClickThroughRate: number;
};
export type LeadReq = { CampaignID: number; Leads: Lead[] };
export type UserSettingsReq = {
  EmailTimeOffset: number; // Time To Wait Between Emails in Seconds
  EmailsPerDay: number; // Max Number of Emails To send Per day
};

export type Lead = {
  FirstName: string;
  LastName: string;
  Email: string;
  PersonalizedLine: string;
  CampaignID: number;
};
export type EmailSeqReq = {
  EmailSeq: EmailSequence;
};
export type GetCampaignOptions = {
  detailed: boolean;
};
export type EmailSequence = {
  Emails: Email[];
};
export type StartCampaignOptions = { FirstEmailOffset: number };
export type Email = {
  Subject: string;
  From: string;
  Body: string;
  TimeOffset: number;
};

export class EmailDraft {}

export class Time {
  static SECOND = 1;
  static MINUTE = 60 * this.SECOND;
  static HOUR = 60 * this.MINUTE;
  static DAY = this.HOUR * 24;
  static WEEK = this.DAY * 7;
  static MONTH = this.WEEK * 4;
  static YEAR = this.WEEK * 52;
}
