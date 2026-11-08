import type { IHelpContent } from '@/Core/Help/HelpTypes';

export const HelpContentEn: IHelpContent = {
  "Faq": [
    {
      "Id": "g1",
      "Title": "Getting started and signing in",
      "Entries": [
        {
          "Id": "g1-q1",
          "Title": "Where do I find the first setup link?",
          "Body": [
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "A new server has no users. Dhow writes a one-time link to its log, and the link is valid for one hour. Look for the "
                },
                {
                  "Type": "code",
                  "Text": "setup_url"
                },
                {
                  "Type": "text",
                  "Text": " field:"
                }
              ]
            },
            {
              "Type": "code",
              "Text": "docker compose logs dhow | grep setup_url"
            },
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "If the hour has passed or you lost the link, restart Dhow to get a new one. The details are in "
                },
                {
                  "Type": "link",
                  "Doc": "install/troubleshooting.md#finding-the-setup-link",
                  "Kind": "doc",
                  "Text": "Finding the setup link"
                },
                {
                  "Type": "text",
                  "Text": "."
                }
              ]
            }
          ]
        },
        {
          "Id": "g1-q2",
          "Title": "Why does sign-in succeed and then ask me to sign in again?",
          "Body": [
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "The browser did not keep the session cookie. The usual causes:"
                }
              ]
            },
            {
              "Type": "list",
              "Ordered": false,
              "Items": [
                [
                  {
                    "Type": "text",
                    "Text": "You open Dhow over HTTP on an address other than "
                  },
                  {
                    "Type": "code",
                    "Text": "localhost"
                  },
                  {
                    "Type": "text",
                    "Text": ". The cookie needs HTTPS."
                  }
                ],
                [
                  {
                    "Type": "text",
                    "Text": "Dhow runs under a sub-path such as "
                  },
                  {
                    "Type": "code",
                    "Text": "/dhow/"
                  },
                  {
                    "Type": "text",
                    "Text": ". Give it its own domain name."
                  }
                ],
                [
                  {
                    "Type": "text",
                    "Text": "A reverse proxy rewrites the "
                  },
                  {
                    "Type": "code",
                    "Text": "Set-Cookie"
                  },
                  {
                    "Type": "text",
                    "Text": " header."
                  }
                ]
              ]
            },
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "The full explanation is in "
                },
                {
                  "Type": "link",
                  "Doc": "install/troubleshooting.md#sign-in-succeeds-but-you-are-not-signed-in",
                  "Kind": "doc",
                  "Text": "Sign-in succeeds but you are not signed in"
                },
                {
                  "Type": "text",
                  "Text": "."
                }
              ]
            }
          ]
        },
        {
          "Id": "g1-q3",
          "Title": "Why does the audit log show the proxy's address for every user?",
          "Body": [
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "Dhow does not trust the proxy in front of it, so it records the address of the connection, which is the proxy. Add the proxy's address to "
                },
                {
                  "Type": "code",
                  "Text": "DHOW_TRUSTED_PROXIES"
                },
                {
                  "Type": "text",
                  "Text": " and restart Dhow. See "
                },
                {
                  "Type": "link",
                  "Doc": "install/reverse-proxy.md",
                  "Kind": "doc",
                  "Text": "Trusted proxies"
                },
                {
                  "Type": "text",
                  "Text": "."
                }
              ]
            }
          ]
        },
        {
          "Id": "g1-q4",
          "Title": "How do I change the interface language or theme?",
          "Body": [
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "Language: press the language button in the top bar. Theme: open "
                },
                {
                  "Type": "strong",
                  "Text": "Preferences"
                },
                {
                  "Type": "text",
                  "Text": " from your account menu. Dhow saves the language and theme for your account, so you see them on any device."
                }
              ]
            }
          ]
        }
      ]
    },
    {
      "Id": "g2",
      "Title": "Runs and deployments",
      "Entries": [
        {
          "Id": "g2-q1",
          "Title": "Why do my runs stay in the queue?",
          "Body": [
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "A queued run needs an active runner in the organization. The runner must have every label the run asks for, and it must support the run's isolation level. Open the run page to see why each runner cannot take it. If no runner fits, register one with the right labels. See "
                },
                {
                  "Type": "link",
                  "Doc": "guides/jobs.md",
                  "Kind": "doc",
                  "Text": "Follow runs"
                },
                {
                  "Type": "text",
                  "Text": " and "
                },
                {
                  "Type": "link",
                  "Doc": "runner/register.md",
                  "Kind": "doc",
                  "Text": "Register a runner"
                },
                {
                  "Type": "text",
                  "Text": "."
                }
              ]
            }
          ]
        },
        {
          "Id": "g2-q2",
          "Title": "What happens if a run waits for a long time?",
          "Body": [
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "A run that is still waiting after 24 hours fails with the code "
                },
                {
                  "Type": "code",
                  "Text": "QUEUE_TIMEOUT"
                },
                {
                  "Type": "text",
                  "Text": "."
                }
              ]
            }
          ]
        },
        {
          "Id": "g2-q3",
          "Title": "Why is a deployment waiting for approval?",
          "Body": [
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "An environment with reviewers asks for approval before it deploys. A reviewer can approve or reject from the "
                },
                {
                  "Type": "strong",
                  "Text": "Deploy approvals"
                },
                {
                  "Type": "text",
                  "Text": " page, and every decision needs a second verification step. The person who started the run cannot approve it, unless an admin allows that. A request that nobody decides expires, and the deployment fails. See "
                },
                {
                  "Type": "link",
                  "Doc": "guides/environments.md",
                  "Kind": "doc",
                  "Text": "Environments"
                },
                {
                  "Type": "text",
                  "Text": "."
                }
              ]
            }
          ]
        },
        {
          "Id": "g2-q4",
          "Title": "Where do I find the log of a failed run?",
          "Body": [
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "Open the run from the "
                },
                {
                  "Type": "strong",
                  "Text": "Jobs"
                },
                {
                  "Type": "text",
                  "Text": " page. The run page shows why it failed in plain words, and the log of every attempt. You can search the log and download it. See "
                },
                {
                  "Type": "link",
                  "Doc": "guides/jobs.md",
                  "Kind": "doc",
                  "Text": "Follow runs"
                },
                {
                  "Type": "text",
                  "Text": "."
                }
              ]
            }
          ]
        }
      ]
    },
    {
      "Id": "g3",
      "Title": "Secrets and notifications",
      "Entries": [
        {
          "Id": "g3-q1",
          "Title": "Can I read a secret's value after I save it?",
          "Body": [
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "No. Dhow stores a secret encrypted and never shows it again, even to the organization owner. If you forget the value, write a new one. See "
                },
                {
                  "Type": "link",
                  "Doc": "guides/secrets.md",
                  "Kind": "doc",
                  "Text": "Secrets and variables"
                },
                {
                  "Type": "text",
                  "Text": "."
                }
              ]
            }
          ]
        },
        {
          "Id": "g3-q2",
          "Title": "How do I fix a notification channel that stopped?",
          "Body": [
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "Dhow stops a channel after 10 failed attempts in a row, or when its owner leaves the organization. Read "
                },
                {
                  "Type": "code",
                  "Text": "DisabledReason"
                },
                {
                  "Type": "text",
                  "Text": " on the channel and fix the cause. Then an admin turns the channel on again by updating it with "
                },
                {
                  "Type": "code",
                  "Text": "Enable: true"
                },
                {
                  "Type": "text",
                  "Text": ". See "
                },
                {
                  "Type": "link",
                  "Doc": "guides/notifications.md",
                  "Kind": "doc",
                  "Text": "Notifications"
                },
                {
                  "Type": "text",
                  "Text": "."
                }
              ]
            }
          ]
        }
      ]
    },
    {
      "Id": "g4",
      "Title": "About Dhow",
      "Entries": [
        {
          "Id": "g4-q1",
          "Title": "How do I know which version I am using?",
          "Body": [
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "Open the "
                },
                {
                  "Type": "strong",
                  "Text": "About Dhow"
                },
                {
                  "Type": "text",
                  "Text": " page. It shows the version, the server's health and checks, and the license. See "
                },
                {
                  "Type": "link",
                  "Doc": "guides/about-and-doctor.md",
                  "Kind": "doc",
                  "Text": "About Dhow and server checks"
                },
                {
                  "Type": "text",
                  "Text": "."
                }
              ]
            }
          ]
        }
      ]
    }
  ],
  "Troubleshooting": [
    {
      "Id": "t1",
      "Title": "Sign-in succeeds but you are not signed in",
      "Body": [
        {
          "Type": "paragraph",
          "Inline": [
            {
              "Type": "text",
              "Text": "The sign-in request answers "
            },
            {
              "Type": "code",
              "Text": "200"
            },
            {
              "Type": "text",
              "Text": ", yet the next page asks you to sign in again. The browser did not store the session cookie "
            },
            {
              "Type": "code",
              "Text": "__Host-dhow_session"
            },
            {
              "Type": "text",
              "Text": ". The browser's developer tools show the "
            },
            {
              "Type": "code",
              "Text": "Set-Cookie"
            },
            {
              "Type": "text",
              "Text": " header with a warning next to it."
            }
          ]
        },
        {
          "Type": "list",
          "Ordered": false,
          "Items": [
            [
              {
                "Type": "strong",
                "Text": "Plain HTTP."
              },
              {
                "Type": "text",
                "Text": " The cookie is always "
              },
              {
                "Type": "code",
                "Text": "Secure"
              },
              {
                "Type": "text",
                "Text": ", so browsers store it only over HTTPS, or on "
              },
              {
                "Type": "code",
                "Text": "http://localhost"
              },
              {
                "Type": "text",
                "Text": " and "
              },
              {
                "Type": "code",
                "Text": "http://127.0.0.1"
              },
              {
                "Type": "text",
                "Text": ", which Chrome and Firefox treat as secure. It is never stored over plain HTTP on another address, such as "
              },
              {
                "Type": "code",
                "Text": "http://192.168.1.20:8080"
              },
              {
                "Type": "text",
                "Text": ". Use the HTTPS address through the proxy."
              }
            ],
            [
              {
                "Type": "strong",
                "Text": "A sub-path."
              },
              {
                "Type": "text",
                "Text": " The "
              },
              {
                "Type": "code",
                "Text": "__Host-"
              },
              {
                "Type": "text",
                "Text": " prefix requires "
              },
              {
                "Type": "code",
                "Text": "Path=/"
              },
              {
                "Type": "text",
                "Text": ", so Dhow cannot work under "
              },
              {
                "Type": "code",
                "Text": "https://example.com/dhow/"
              },
              {
                "Type": "text",
                "Text": ". Give it its own host name."
              }
            ],
            [
              {
                "Type": "strong",
                "Text": "A shared domain."
              },
              {
                "Type": "text",
                "Text": " The cookie has no "
              },
              {
                "Type": "code",
                "Text": "Domain"
              },
              {
                "Type": "text",
                "Text": ", so it belongs to exactly the host that set it and is never shared with other sub-domains. That is by design. Open Dhow on the host name of "
              },
              {
                "Type": "code",
                "Text": "DHOW_PUBLIC_URL"
              },
              {
                "Type": "text",
                "Text": "."
              }
            ],
            [
              {
                "Type": "strong",
                "Text": "A proxy that changes cookies."
              },
              {
                "Type": "text",
                "Text": " Remove any "
              },
              {
                "Type": "code",
                "Text": "proxy_cookie_path"
              },
              {
                "Type": "text",
                "Text": ", "
              },
              {
                "Type": "code",
                "Text": "proxy_cookie_domain"
              },
              {
                "Type": "text",
                "Text": " or "
              },
              {
                "Type": "code",
                "Text": "proxy_cookie_flags"
              },
              {
                "Type": "text",
                "Text": " in nginx, or any rule that rewrites "
              },
              {
                "Type": "code",
                "Text": "Set-Cookie"
              },
              {
                "Type": "text",
                "Text": " in other proxies, for the Dhow site."
              }
            ]
          ]
        }
      ]
    },
    {
      "Id": "t2",
      "Title": "The audit log shows the proxy's address for everyone",
      "Body": [
        {
          "Type": "paragraph",
          "Inline": [
            {
              "Type": "text",
              "Text": "The sign-in events in the audit log and the "
            },
            {
              "Type": "code",
              "Text": "client_ip"
            },
            {
              "Type": "text",
              "Text": " field in the server log show the same address, such as "
            },
            {
              "Type": "code",
              "Text": "127.0.0.1"
            },
            {
              "Type": "text",
              "Text": " or "
            },
            {
              "Type": "code",
              "Text": "172.18.0.1"
            },
            {
              "Type": "text",
              "Text": ", for every user. Dhow does not trust the proxy in front of it, so it uses the address of the connection, which is the proxy."
            }
          ]
        },
        {
          "Type": "paragraph",
          "Inline": [
            {
              "Type": "text",
              "Text": "Add the proxy's address, as Dhow sees it, to "
            },
            {
              "Type": "code",
              "Text": "DHOW_TRUSTED_PROXIES"
            },
            {
              "Type": "text",
              "Text": " and restart Dhow. "
            },
            {
              "Type": "link",
              "Doc": "install/reverse-proxy.md#trusted-proxies",
              "Kind": "doc",
              "Text": "Trusted proxies"
            },
            {
              "Type": "text",
              "Text": " has the value for each setup. With the Compose stack and a proxy on the host, it is the Docker bridge gateway, not "
            },
            {
              "Type": "code",
              "Text": "127.0.0.1"
            },
            {
              "Type": "text",
              "Text": "."
            }
          ]
        },
        {
          "Type": "paragraph",
          "Inline": [
            {
              "Type": "text",
              "Text": "If the address shown belongs to a CDN such as Cloudflare, add the CDN's ranges too, see "
            },
            {
              "Type": "link",
              "Doc": "install/reverse-proxy.md#a-chain-of-proxies-cloudflare-in-front-of-nginx",
              "Kind": "doc",
              "Text": "A chain of proxies"
            },
            {
              "Type": "text",
              "Text": "."
            }
          ]
        },
        {
          "Type": "paragraph",
          "Inline": [
            {
              "Type": "text",
              "Text": "Events written before the fix keep the address they were recorded with: the audit log cannot be changed."
            }
          ]
        }
      ]
    },
    {
      "Id": "t3",
      "Title": "Nobody can sign in, even with the right password",
      "Body": [
        {
          "Type": "paragraph",
          "Inline": [
            {
              "Type": "text",
              "Text": "Every sign-in answers "
            },
            {
              "Type": "code",
              "Text": "INVALID_CREDENTIALS"
            },
            {
              "Type": "text",
              "Text": " with a "
            },
            {
              "Type": "code",
              "Text": "Retry-After"
            },
            {
              "Type": "text",
              "Text": " header, for every user at the same time. Dhow throttles failed sign-ins per account and per client address: after 20 failures from one address, that address is locked for 30 seconds, and each further failure doubles the lock up to 15 minutes."
            }
          ]
        },
        {
          "Type": "paragraph",
          "Inline": [
            {
              "Type": "text",
              "Text": "When "
            },
            {
              "Type": "code",
              "Text": "DHOW_TRUSTED_PROXIES"
            },
            {
              "Type": "text",
              "Text": " is missing the proxy, all users share the proxy's address, so a few people mistyping their passwords lock out everybody. Check the server log: if every "
            },
            {
              "Type": "code",
              "Text": "POST /api/v1/sessions"
            },
            {
              "Type": "text",
              "Text": " line has the same "
            },
            {
              "Type": "code",
              "Text": "client_ip"
            },
            {
              "Type": "text",
              "Text": ", fix "
            },
            {
              "Type": "code",
              "Text": "DHOW_TRUSTED_PROXIES"
            },
            {
              "Type": "text",
              "Text": " as in "
            },
            {
              "Type": "link",
              "Doc": "install/troubleshooting.md#the-audit-log-shows-the-proxys-address-for-everyone",
              "Kind": "doc",
              "Text": "the previous section"
            },
            {
              "Type": "text",
              "Text": " and restart Dhow. The lock on the proxy's address then no longer applies to anyone. Locks on single accounts, after 5 failures for the same account, stay until they expire. Every lock expires within 15 minutes, and attempts made while a lock is active never extend it."
            }
          ]
        }
      ]
    },
    {
      "Id": "t4",
      "Title": "429 RATE_LIMITED on sign-in pages or invitations",
      "Body": [
        {
          "Type": "paragraph",
          "Inline": [
            {
              "Type": "text",
              "Text": "The answer carries "
            },
            {
              "Type": "code",
              "Text": "Retry-After"
            },
            {
              "Type": "text",
              "Text": ", the seconds until the counter starts again. Dhow limits the public sign-in routes per client network and invitations per member and per organization, see "
            },
            {
              "Type": "link",
              "Doc": "install/configuration.md#rate-limits",
              "Kind": "doc",
              "Text": "Rate limits"
            },
            {
              "Type": "text",
              "Text": "."
            }
          ]
        },
        {
          "Type": "list",
          "Ordered": false,
          "Items": [
            [
              {
                "Type": "strong",
                "Text": "Everyone gets it at once."
              },
              {
                "Type": "text",
                "Text": " All users probably share one address: fix "
              },
              {
                "Type": "code",
                "Text": "DHOW_TRUSTED_PROXIES"
              },
              {
                "Type": "text",
                "Text": " as in "
              },
              {
                "Type": "link",
                "Doc": "install/troubleshooting.md#the-audit-log-shows-the-proxys-address-for-everyone",
                "Kind": "doc",
                "Text": "The audit log shows the proxy's address for everyone"
              },
              {
                "Type": "text",
                "Text": "."
              }
            ],
            [
              {
                "Type": "strong",
                "Text": "A large office behind one NAT."
              },
              {
                "Type": "text",
                "Text": " Raise "
              },
              {
                "Type": "code",
                "Text": "DHOW_RATE_LIMIT_PUBLIC"
              },
              {
                "Type": "text",
                "Text": " and restart Dhow."
              }
            ],
            [
              {
                "Type": "strong",
                "Text": "Inviting a whole team."
              },
              {
                "Type": "text",
                "Text": " Raise "
              },
              {
                "Type": "code",
                "Text": "DHOW_RATE_LIMIT_INVITES_USER"
              },
              {
                "Type": "text",
                "Text": " and "
              },
              {
                "Type": "code",
                "Text": "DHOW_RATE_LIMIT_INVITES_ORG"
              },
              {
                "Type": "text",
                "Text": ", or wait for the next hour. An invitation that answers "
              },
              {
                "Type": "code",
                "Text": "INVITATION_SEND_LIMIT"
              },
              {
                "Type": "text",
                "Text": " was sent 5 times: revoke it and invite again."
              }
            ]
          ]
        }
      ]
    },
    {
      "Id": "t5",
      "Title": "Finding the setup link",
      "Body": [
        {
          "Type": "paragraph",
          "Inline": [
            {
              "Type": "text",
              "Text": "A new instance has no users. At every start without an owner, Dhow logs a one-time link that is valid for one hour:"
            }
          ]
        },
        {
          "Type": "code",
          "Text": "docker compose logs dhow | grep setup_url"
        },
        {
          "Type": "paragraph",
          "Inline": [
            {
              "Type": "text",
              "Text": "Without Compose, look for the "
            },
            {
              "Type": "code",
              "Text": "setup_url"
            },
            {
              "Type": "text",
              "Text": " field in the server's output. The link is "
            },
            {
              "Type": "code",
              "Text": "DHOW_PUBLIC_URL"
            },
            {
              "Type": "text",
              "Text": " followed by "
            },
            {
              "Type": "code",
              "Text": "/setup#"
            },
            {
              "Type": "text",
              "Text": " and the token, so it only works if the public URL is right and the proxy is in place."
            }
          ]
        },
        {
          "Type": "list",
          "Ordered": false,
          "Items": [
            [
              {
                "Type": "strong",
                "Text": "The hour is over, or the link is lost."
              },
              {
                "Type": "text",
                "Text": " Restart Dhow, for example with "
              },
              {
                "Type": "code",
                "Text": "docker compose restart dhow"
              },
              {
                "Type": "text",
                "Text": ", to get a new link. The old one stops working at once."
              }
            ],
            [
              {
                "Type": "strong",
                "Text": "SETUP_TOKEN_INVALID."
              },
              {
                "Type": "text",
                "Text": " The link has expired, was replaced by a newer one, or was copied without the part after "
              },
              {
                "Type": "code",
                "Text": "#"
              },
              {
                "Type": "text",
                "Text": "."
              }
            ],
            [
              {
                "Type": "strong",
                "Text": "SETUP_COMPLETED."
              },
              {
                "Type": "text",
                "Text": " Setup is done and the link is gone for good. Sign in with the owner account."
              }
            ],
            [
              {
                "Type": "strong",
                "Text": "No setup_url line at all."
              },
              {
                "Type": "text",
                "Text": " The instance already has an owner, so there is nothing to set up."
              }
            ]
          ]
        },
        {
          "Type": "paragraph",
          "Inline": [
            {
              "Type": "link",
              "Doc": "install/compose.md#create-the-first-owner",
              "Kind": "doc",
              "Text": "Create the first owner"
            },
            {
              "Type": "text",
              "Text": " describes the setup page and the equivalent API call."
            }
          ]
        }
      ]
    }
  ],
  "Releases": [
    {
      "Version": "0.1.0",
      "Title": "Dhow 0.1.0",
      "Sections": [
        {
          "Kind": "highlights",
          "Title": "Highlights",
          "Body": [
            {
              "Type": "list",
              "Ordered": false,
              "Items": [
                [
                  {
                    "Type": "strong",
                    "Text": "Help inside the app."
                  },
                  {
                    "Type": "text",
                    "Text": " The header of every page has a "
                  },
                  {
                    "Type": "strong",
                    "Text": "Help"
                  },
                  {
                    "Type": "text",
                    "Text": " button that opens the page's documentation in the interface language, and a short line that explains the page."
                  }
                ],
                [
                  {
                    "Type": "strong",
                    "Text": "A frequently asked questions page."
                  },
                  {
                    "Type": "text",
                    "Text": " It works without an internet connection, and it has search. Find it in "
                  },
                  {
                    "Type": "strong",
                    "Text": "Help"
                  },
                  {
                    "Type": "text",
                    "Text": "."
                  }
                ],
                [
                  {
                    "Type": "strong",
                    "Text": "About Dhow."
                  },
                  {
                    "Type": "text",
                    "Text": " A page that shows the version, the server's health and checks, and the license."
                  }
                ],
                [
                  {
                    "Type": "strong",
                    "Text": "Notifications."
                  },
                  {
                    "Type": "text",
                    "Text": " Dhow sends an email or a signed webhook when a deployment fails or waits for approval. See "
                  },
                  {
                    "Type": "link",
                    "Doc": "guides/notifications.md",
                    "Kind": "doc",
                    "Text": "Notifications"
                  },
                  {
                    "Type": "text",
                    "Text": "."
                  }
                ],
                [
                  {
                    "Type": "strong",
                    "Text": "The run log."
                  },
                  {
                    "Type": "text",
                    "Text": " Search the log, link to a specific line, and download it. See "
                  },
                  {
                    "Type": "link",
                    "Doc": "guides/jobs.md",
                    "Kind": "doc",
                    "Text": "Follow runs"
                  },
                  {
                    "Type": "text",
                    "Text": "."
                  }
                ]
              ]
            }
          ]
        }
      ]
    }
  ]
};
