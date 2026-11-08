import type { IHelpContent } from '@/Core/Help/HelpTypes';

export const HelpContentAr: IHelpContent = {
  "Faq": [
    {
      "Id": "g1",
      "Title": "البدء والدخول",
      "Entries": [
        {
          "Id": "g1-q1",
          "Title": "أين أجد رابط الإعداد الأول؟",
          "Body": [
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "السيرفر الجديد ليس فيه مستخدمون. يكتب داو في السجل رابطًا يعمل مرة واحدة، وصلاحيته ساعة. ابحث عن الحقل "
                },
                {
                  "Type": "code",
                  "Text": "setup_url"
                },
                {
                  "Type": "text",
                  "Text": ":"
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
                  "Text": "إذا انتهت الساعة أو ضاع الرابط، أعد تشغيل داو لتحصل على رابط جديد. التفاصيل في "
                },
                {
                  "Type": "link",
                  "Doc": "install/troubleshooting.md#العثور-على-رابط-الإعداد",
                  "Kind": "doc",
                  "Text": "العثور على رابط الإعداد"
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
          "Title": "لماذا ينجح تسجيل الدخول ثم يطلب مني الدخول مرة أخرى؟",
          "Body": [
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "المتصفح لم يحفظ كوكي الجلسة. الأسباب الشائعة:"
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
                    "Text": "تفتح داو عبر HTTP على عنوان غير "
                  },
                  {
                    "Type": "code",
                    "Text": "localhost"
                  },
                  {
                    "Type": "text",
                    "Text": ". الكوكي يحتاج إلى HTTPS."
                  }
                ],
                [
                  {
                    "Type": "text",
                    "Text": "داو يعمل تحت مسار فرعي مثل "
                  },
                  {
                    "Type": "code",
                    "Text": "/dhow/"
                  },
                  {
                    "Type": "text",
                    "Text": ". أعطه اسم نطاق خاصًا به."
                  }
                ],
                [
                  {
                    "Type": "text",
                    "Text": "وكيل عكسي يغيّر ترويسة "
                  },
                  {
                    "Type": "code",
                    "Text": "Set-Cookie"
                  },
                  {
                    "Type": "text",
                    "Text": "."
                  }
                ]
              ]
            },
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "الشرح الكامل في "
                },
                {
                  "Type": "link",
                  "Doc": "install/troubleshooting.md#ينجح-تسجيل-الدخول-لكن-الجلسة-لا-تبقى",
                  "Kind": "doc",
                  "Text": "ينجح تسجيل الدخول لكن الجلسة لا تبقى"
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
          "Title": "لماذا يظهر عنوان الوكيل لكل المستخدمين في سجل الأحداث؟",
          "Body": [
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "لأن داو لا يثق بالوكيل الذي أمامه، فيسجّل عنوان الاتصال، وهو عنوان الوكيل. أضف عنوان الوكيل إلى "
                },
                {
                  "Type": "code",
                  "Text": "DHOW_TRUSTED_PROXIES"
                },
                {
                  "Type": "text",
                  "Text": " وأعد تشغيل داو. راجع "
                },
                {
                  "Type": "link",
                  "Doc": "install/reverse-proxy.md",
                  "Kind": "doc",
                  "Text": "الوكلاء الموثوقون"
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
          "Title": "كيف أغيّر لغة الواجهة أو ثيمها؟",
          "Body": [
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "اللغة: اضغط زر اللغة في الشريط العلوي. الثيم: افتح "
                },
                {
                  "Type": "strong",
                  "Text": "التفضيلات"
                },
                {
                  "Type": "text",
                  "Text": " من قائمة حسابك. داو يحفظ اللغة والثيم لحسابك، فتجدهما كما هما على أي جهاز."
                }
              ]
            }
          ]
        }
      ]
    },
    {
      "Id": "g2",
      "Title": "التشغيلات والنشر",
      "Entries": [
        {
          "Id": "g2-q1",
          "Title": "لماذا تبقى تشغيلاتي في الانتظار؟",
          "Body": [
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "التشغيل المنتظر يحتاج إلى جهاز تشغيل (runner) نشط في المؤسسة، وفيه كل الأوسمة التي يطلبها التشغيل، ويدعم مستوى العزل الخاص به. افتح صفحة التشغيل لترى سبب الانتظار لكل جهاز. إذا لم يوجد جهاز مناسب، سجّل واحدًا بالأوسمة الصحيحة. راجع "
                },
                {
                  "Type": "link",
                  "Doc": "guides/jobs.md",
                  "Kind": "doc",
                  "Text": "تابع التشغيلات"
                },
                {
                  "Type": "text",
                  "Text": " و"
                },
                {
                  "Type": "link",
                  "Doc": "runner/register.md",
                  "Kind": "doc",
                  "Text": "تسجيل جهاز تشغيل"
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
          "Title": "ماذا يحدث إذا بقي التشغيل في الانتظار طويلًا؟",
          "Body": [
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "التشغيل الذي ما زال ينتظر بعد 24 ساعة يفشل بالرمز "
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
          "Title": "لماذا ينتظر النشر موافقة؟",
          "Body": [
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "البيئة التي فيها مراجعون تطلب موافقة قبل النشر. يستطيع المراجع أن يوافق أو يرفض من صفحة "
                },
                {
                  "Type": "strong",
                  "Text": "موافقات النشر"
                },
                {
                  "Type": "text",
                  "Text": "، وكل قرار يحتاج إلى خطوة ثانية للتحقق. من بدأ التنفيذ لا يوافق عليه بنفسه، إلا إذا سمح المدير بذلك. الطلب الذي لا يقرر فيه أحد ينتهي، ويفشل النشر. راجع "
                },
                {
                  "Type": "link",
                  "Doc": "guides/environments.md",
                  "Kind": "doc",
                  "Text": "البيئات"
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
          "Title": "أين أجد سجل تشغيل فشل؟",
          "Body": [
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "افتح التشغيل من صفحة "
                },
                {
                  "Type": "strong",
                  "Text": "التشغيلات"
                },
                {
                  "Type": "text",
                  "Text": ". صفحة التشغيل تعرض سبب الفشل بكلمات واضحة، وسجل كل محاولة. يمكنك البحث في السجل وتنزيله. راجع "
                },
                {
                  "Type": "link",
                  "Doc": "guides/jobs.md",
                  "Kind": "doc",
                  "Text": "تابع التشغيلات"
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
      "Title": "الأسرار والإشعارات",
      "Entries": [
        {
          "Id": "g3-q1",
          "Title": "هل أستطيع قراءة قيمة سر بعد حفظه؟",
          "Body": [
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "لا. داو يحفظ السر مشفّرًا ولا يعرضه مرة أخرى، حتى لصاحب المؤسسة. إذا نسيت القيمة، اكتب قيمة جديدة. راجع "
                },
                {
                  "Type": "link",
                  "Doc": "guides/secrets.md",
                  "Kind": "doc",
                  "Text": "الأسرار والمتغيّرات"
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
          "Title": "كيف أصلح قناة إشعارات توقفت؟",
          "Body": [
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "داو يوقف القناة بعد 10 محاولات فاشلة متتالية، أو عندما يغادر صاحبها المؤسسة. اقرأ "
                },
                {
                  "Type": "code",
                  "Text": "DisabledReason"
                },
                {
                  "Type": "text",
                  "Text": " في القناة، وأصلح السبب، ثم يعيد المدير تشغيلها بتحديثها بـ "
                },
                {
                  "Type": "code",
                  "Text": "Enable: true"
                },
                {
                  "Type": "text",
                  "Text": ". راجع "
                },
                {
                  "Type": "link",
                  "Doc": "guides/notifications.md",
                  "Kind": "doc",
                  "Text": "الإشعارات"
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
      "Title": "عن داو",
      "Entries": [
        {
          "Id": "g4-q1",
          "Title": "كيف أعرف النسخة التي أستخدمها؟",
          "Body": [
            {
              "Type": "paragraph",
              "Inline": [
                {
                  "Type": "text",
                  "Text": "افتح صفحة "
                },
                {
                  "Type": "strong",
                  "Text": "عن داو"
                },
                {
                  "Type": "text",
                  "Text": ". تعرض النسخة وحالة السيرفر وفحوصاته، والترخيص. راجع "
                },
                {
                  "Type": "link",
                  "Doc": "guides/about-and-doctor.md",
                  "Kind": "doc",
                  "Text": "عن داو وفحوصات السيرفر"
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
      "Title": "ينجح تسجيل الدخول لكن الجلسة لا تبقى",
      "Body": [
        {
          "Type": "paragraph",
          "Inline": [
            {
              "Type": "text",
              "Text": "طلب تسجيل الدخول يرد بـ "
            },
            {
              "Type": "code",
              "Text": "200"
            },
            {
              "Type": "text",
              "Text": "، لكن الصفحة التالية تطلب منك الدخول مرة أخرى. لم يحفظ المتصفح كوكي الجلسة "
            },
            {
              "Type": "code",
              "Text": "__Host-dhow_session"
            },
            {
              "Type": "text",
              "Text": ". أدوات المطوّر في المتصفح تعرض ترويسة "
            },
            {
              "Type": "code",
              "Text": "Set-Cookie"
            },
            {
              "Type": "text",
              "Text": " مع تحذير بجوارها."
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
                "Text": "HTTP العادي."
              },
              {
                "Type": "text",
                "Text": " الكوكي دائمًا "
              },
              {
                "Type": "code",
                "Text": "Secure"
              },
              {
                "Type": "text",
                "Text": "، فلا تحفظه المتصفحات إلا عبر HTTPS، أو على "
              },
              {
                "Type": "code",
                "Text": "http://localhost"
              },
              {
                "Type": "text",
                "Text": " و"
              },
              {
                "Type": "code",
                "Text": "http://127.0.0.1"
              },
              {
                "Type": "text",
                "Text": " اللذين يعتبرهما Chrome وFirefox آمنين. لا يُحفظ أبدًا عبر HTTP العادي على عنوان آخر، مثل "
              },
              {
                "Type": "code",
                "Text": "http://192.168.1.20:8080"
              },
              {
                "Type": "text",
                "Text": ". استخدم عنوان HTTPS عبر الوكيل."
              }
            ],
            [
              {
                "Type": "strong",
                "Text": "مسار فرعي."
              },
              {
                "Type": "text",
                "Text": " البادئة "
              },
              {
                "Type": "code",
                "Text": "__Host-"
              },
              {
                "Type": "text",
                "Text": " تشترط "
              },
              {
                "Type": "code",
                "Text": "Path=/"
              },
              {
                "Type": "text",
                "Text": "، فلا يعمل داو تحت "
              },
              {
                "Type": "code",
                "Text": "https://example.com/dhow/"
              },
              {
                "Type": "text",
                "Text": ". أعطه اسم نطاق خاصًا به."
              }
            ],
            [
              {
                "Type": "strong",
                "Text": "نطاق مشترك."
              },
              {
                "Type": "text",
                "Text": " الكوكي ليس له "
              },
              {
                "Type": "code",
                "Text": "Domain"
              },
              {
                "Type": "text",
                "Text": "، فهو للجهاز الذي ضبطه بالضبط، ولا يُشارك أبدًا مع نطاقات فرعية أخرى. هذا مقصود. افتح داو على اسم الجهاز الموجود في "
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
                "Text": "وكيل يغيّر الكوكيز."
              },
              {
                "Type": "text",
                "Text": " احذف أي "
              },
              {
                "Type": "code",
                "Text": "proxy_cookie_path"
              },
              {
                "Type": "text",
                "Text": " أو "
              },
              {
                "Type": "code",
                "Text": "proxy_cookie_domain"
              },
              {
                "Type": "text",
                "Text": " أو "
              },
              {
                "Type": "code",
                "Text": "proxy_cookie_flags"
              },
              {
                "Type": "text",
                "Text": " في nginx، أو أي قاعدة تعيد كتابة "
              },
              {
                "Type": "code",
                "Text": "Set-Cookie"
              },
              {
                "Type": "text",
                "Text": " في الوكلاء الآخرين، لموقع داو."
              }
            ]
          ]
        }
      ]
    },
    {
      "Id": "t2",
      "Title": "سجل الأحداث يعرض عنوان الوكيل للجميع",
      "Body": [
        {
          "Type": "paragraph",
          "Inline": [
            {
              "Type": "text",
              "Text": "أحداث الدخول في سجل الأحداث، وحقل "
            },
            {
              "Type": "code",
              "Text": "client_ip"
            },
            {
              "Type": "text",
              "Text": " في سجل السيرفر، تعرض العنوان نفسه لكل المستخدمين، مثل "
            },
            {
              "Type": "code",
              "Text": "127.0.0.1"
            },
            {
              "Type": "text",
              "Text": " أو "
            },
            {
              "Type": "code",
              "Text": "172.18.0.1"
            },
            {
              "Type": "text",
              "Text": ". داو لا يثق بالوكيل الذي أمامه، فيستخدم عنوان الاتصال، وهو الوكيل."
            }
          ]
        },
        {
          "Type": "paragraph",
          "Inline": [
            {
              "Type": "text",
              "Text": "أضف عنوان الوكيل، كما يراه داو، إلى "
            },
            {
              "Type": "code",
              "Text": "DHOW_TRUSTED_PROXIES"
            },
            {
              "Type": "text",
              "Text": "، وأعد تشغيل داو. في "
            },
            {
              "Type": "link",
              "Doc": "install/reverse-proxy.md#الوكلاء-الموثوقون",
              "Kind": "doc",
              "Text": "الوكلاء الموثوقون"
            },
            {
              "Type": "text",
              "Text": " القيمة لكل إعداد. مع حزمة Compose ووكيل على الجهاز، القيمة هي بوابة Docker bridge، وليست "
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
              "Text": "إذا كان العنوان الظاهر لشبكة CDN مثل Cloudflare، فأضف نطاقاتها أيضًا. راجع "
            },
            {
              "Type": "link",
              "Doc": "install/reverse-proxy.md#سلسلة-وكلاء-cloudflare-أمام-nginx",
              "Kind": "doc",
              "Text": "سلسلة وكلاء"
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
              "Text": "الأحداث التي كُتبت قبل الإصلاح تبقى بالعنوان الذي سُجّلت به، لأن سجل الأحداث لا يمكن تغييره."
            }
          ]
        }
      ]
    },
    {
      "Id": "t3",
      "Title": "لا أحد يستطيع الدخول حتى بكلمة السر الصحيحة",
      "Body": [
        {
          "Type": "paragraph",
          "Inline": [
            {
              "Type": "text",
              "Text": "كل محاولة دخول ترد بـ "
            },
            {
              "Type": "code",
              "Text": "INVALID_CREDENTIALS"
            },
            {
              "Type": "text",
              "Text": " مع ترويسة "
            },
            {
              "Type": "code",
              "Text": "Retry-After"
            },
            {
              "Type": "text",
              "Text": "، لكل المستخدمين في الوقت نفسه. يحدّ داو محاولات الدخول الفاشلة لكل حساب ولكل عنوان عميل. بعد 20 محاولة فاشلة من عنوان واحد، يُقفل هذا العنوان 30 ثانية، وكل فشل بعد ذلك يضاعف القفل حتى 15 دقيقة."
            }
          ]
        },
        {
          "Type": "paragraph",
          "Inline": [
            {
              "Type": "text",
              "Text": "إذا لم يكن الوكيل في "
            },
            {
              "Type": "code",
              "Text": "DHOW_TRUSTED_PROXIES"
            },
            {
              "Type": "text",
              "Text": "، يشترك كل المستخدمين في عنوان الوكيل. لذلك يكفي أن يخطئ بضعة أشخاص في كلمات سرهم ليُقفل الدخول على الجميع. افحص سجل السيرفر: إذا كان لكل سطر "
            },
            {
              "Type": "code",
              "Text": "POST /api/v1/sessions"
            },
            {
              "Type": "text",
              "Text": " القيمة نفسها في "
            },
            {
              "Type": "code",
              "Text": "client_ip"
            },
            {
              "Type": "text",
              "Text": "، فأصلح "
            },
            {
              "Type": "code",
              "Text": "DHOW_TRUSTED_PROXIES"
            },
            {
              "Type": "text",
              "Text": " كما في "
            },
            {
              "Type": "link",
              "Doc": "install/troubleshooting.md#سجل-الأحداث-يعرض-عنوان-الوكيل-للجميع",
              "Kind": "doc",
              "Text": "القسم السابق"
            },
            {
              "Type": "text",
              "Text": "، وأعد تشغيل داو. عندها لا يعود القفل على عنوان الوكيل يطبّق على أحد. أما القفل على حساب واحد، بعد 5 محاولات فاشلة للحساب نفسه، فيبقى حتى تنتهي مدته. كل قفل ينتهي خلال 15 دقيقة، والمحاولات أثناء القفل لا تطيله أبدًا."
            }
          ]
        }
      ]
    },
    {
      "Id": "t4",
      "Title": "429 RATE_LIMITED في صفحات الدخول أو الدعوات",
      "Body": [
        {
          "Type": "paragraph",
          "Inline": [
            {
              "Type": "text",
              "Text": "يحمل الرد "
            },
            {
              "Type": "code",
              "Text": "Retry-After"
            },
            {
              "Type": "text",
              "Text": "، وهو عدد الثواني حتى يبدأ العدّاد من جديد. يحدّ داو مسارات الدخول العامة لكل شبكة عميل، والدعوات لكل عضو ولكل مؤسسة. راجع "
            },
            {
              "Type": "link",
              "Doc": "install/configuration.md#حدود-الطلبات",
              "Kind": "doc",
              "Text": "حدود الطلبات"
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
                "Text": "يظهر للجميع في الوقت نفسه."
              },
              {
                "Type": "text",
                "Text": " الأرجح أن كل المستخدمين يشتركون في عنوان واحد. أصلح "
              },
              {
                "Type": "code",
                "Text": "DHOW_TRUSTED_PROXIES"
              },
              {
                "Type": "text",
                "Text": " كما في "
              },
              {
                "Type": "link",
                "Doc": "install/troubleshooting.md#سجل-الأحداث-يعرض-عنوان-الوكيل-للجميع",
                "Kind": "doc",
                "Text": "سجل الأحداث يعرض عنوان الوكيل للجميع"
              },
              {
                "Type": "text",
                "Text": "."
              }
            ],
            [
              {
                "Type": "strong",
                "Text": "مكتب كبير خلف NAT واحد."
              },
              {
                "Type": "text",
                "Text": " ارفع "
              },
              {
                "Type": "code",
                "Text": "DHOW_RATE_LIMIT_PUBLIC"
              },
              {
                "Type": "text",
                "Text": " وأعد تشغيل داو."
              }
            ],
            [
              {
                "Type": "strong",
                "Text": "دعوة فريق كامل."
              },
              {
                "Type": "text",
                "Text": " ارفع "
              },
              {
                "Type": "code",
                "Text": "DHOW_RATE_LIMIT_INVITES_USER"
              },
              {
                "Type": "text",
                "Text": " و"
              },
              {
                "Type": "code",
                "Text": "DHOW_RATE_LIMIT_INVITES_ORG"
              },
              {
                "Type": "text",
                "Text": "، أو انتظر الساعة التالية. الدعوة التي ترد بـ "
              },
              {
                "Type": "code",
                "Text": "INVITATION_SEND_LIMIT"
              },
              {
                "Type": "text",
                "Text": " أُرسلت 5 مرات: ألغِها وادعُ مرة أخرى."
              }
            ]
          ]
        }
      ]
    },
    {
      "Id": "t5",
      "Title": "العثور على رابط الإعداد",
      "Body": [
        {
          "Type": "paragraph",
          "Inline": [
            {
              "Type": "text",
              "Text": "السيرفر الجديد ليس فيه مستخدمون. في كل تشغيل من غير صاحب نسخة، يكتب داو في السجل رابطًا يعمل مرة واحدة، وصلاحيته ساعة:"
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
              "Text": "بدون Compose، ابحث عن الحقل "
            },
            {
              "Type": "code",
              "Text": "setup_url"
            },
            {
              "Type": "text",
              "Text": " في مخرجات السيرفر. الرابط هو "
            },
            {
              "Type": "code",
              "Text": "DHOW_PUBLIC_URL"
            },
            {
              "Type": "text",
              "Text": " ثم "
            },
            {
              "Type": "code",
              "Text": "/setup#"
            },
            {
              "Type": "text",
              "Text": " ثم الرمز. لذلك لا يعمل إلا إذا كان العنوان العام صحيحًا والوكيل في مكانه."
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
                "Text": "انتهت الساعة، أو ضاع الرابط."
              },
              {
                "Type": "text",
                "Text": " أعد تشغيل داو، مثلًا بـ "
              },
              {
                "Type": "code",
                "Text": "docker compose restart dhow"
              },
              {
                "Type": "text",
                "Text": "، لتحصل على رابط جديد. الرابط القديم يتوقف فورًا."
              }
            ],
            [
              {
                "Type": "strong",
                "Text": "SETUP_TOKEN_INVALID."
              },
              {
                "Type": "text",
                "Text": " انتهت صلاحية الرابط، أو استُبدل برابط أحدث، أو نُسخ بدون الجزء الذي بعد "
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
                "Text": " اكتمل الإعداد واختفى الرابط نهائيًا. سجّل الدخول بحساب صاحب النسخة."
              }
            ],
            [
              {
                "Type": "strong",
                "Text": "لا يوجد سطر setup_url أبدًا."
              },
              {
                "Type": "text",
                "Text": " للسيرفر صاحب نسخة بالفعل، فلا يوجد ما تُعدّه."
              }
            ]
          ]
        },
        {
          "Type": "paragraph",
          "Inline": [
            {
              "Type": "link",
              "Doc": "install/compose.md#أنشئ-حساب-صاحب-النسخة-الأول",
              "Kind": "doc",
              "Text": "أنشئ حساب صاحب النسخة الأول"
            },
            {
              "Type": "text",
              "Text": " يشرح صفحة الإعداد وطلب الـ API المقابل."
            }
          ]
        }
      ]
    }
  ],
  "Releases": [
    {
      "Version": "0.1.0",
      "Title": "داو 0.1.0",
      "Sections": [
        {
          "Kind": "highlights",
          "Title": "أهم التغييرات",
          "Body": [
            {
              "Type": "list",
              "Ordered": false,
              "Items": [
                [
                  {
                    "Type": "strong",
                    "Text": "مساعدة داخل التطبيق."
                  },
                  {
                    "Type": "text",
                    "Text": " في رأس كل صفحة زر "
                  },
                  {
                    "Type": "strong",
                    "Text": "مساعدة"
                  },
                  {
                    "Type": "text",
                    "Text": " يفتح صفحتها في الوثائق بلغة الواجهة، وسطر قصير يشرح الصفحة."
                  }
                ],
                [
                  {
                    "Type": "strong",
                    "Text": "صفحة الأسئلة الشائعة."
                  },
                  {
                    "Type": "text",
                    "Text": " تعمل بدون اتصال بالإنترنت، وفيها بحث. تجدها في "
                  },
                  {
                    "Type": "strong",
                    "Text": "المساعدة"
                  },
                  {
                    "Type": "text",
                    "Text": "."
                  }
                ],
                [
                  {
                    "Type": "strong",
                    "Text": "عن داو."
                  },
                  {
                    "Type": "text",
                    "Text": " صفحة تعرض النسخة وحالة السيرفر وفحوصاته والترخيص."
                  }
                ],
                [
                  {
                    "Type": "strong",
                    "Text": "الإشعارات."
                  },
                  {
                    "Type": "text",
                    "Text": " يرسل داو بريدًا أو webhook موقّعًا عند فشل نشر أو انتظاره موافقة. راجع "
                  },
                  {
                    "Type": "link",
                    "Doc": "guides/notifications.md",
                    "Kind": "doc",
                    "Text": "الإشعارات"
                  },
                  {
                    "Type": "text",
                    "Text": "."
                  }
                ],
                [
                  {
                    "Type": "strong",
                    "Text": "سجل التشغيل."
                  },
                  {
                    "Type": "text",
                    "Text": " بحث في السجل، ورابط لسطر محدد، وتنزيل. راجع "
                  },
                  {
                    "Type": "link",
                    "Doc": "guides/jobs.md",
                    "Kind": "doc",
                    "Text": "تابع التشغيلات"
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
